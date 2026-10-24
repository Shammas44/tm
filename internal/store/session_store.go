package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"taskmanager/internal/model"
)

func (s *Store) StartSession(ctx context.Context, agentID string, activeTaskID *int64) (*model.Session, error) {
	if agentID == "" {
		agentID = "developer"
	}
	sessionID := uuid.NewString()
	now := time.Now().UTC()

	query := `
		INSERT INTO sessions (id, agent_id, active_task_id, status, checkpoint, handoff, summary, next_action, last_checkpoint_at, started_at, ended_at)
		VALUES (?, ?, ?, ?, '', '', '', '', ?, ?, NULL)
	`
	_, err := s.db.ExecContext(ctx, query,
		sessionID,
		agentID,
		activeTaskID,
		string(model.SessionStatusActive),
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	session := &model.Session{
		ID:               sessionID,
		AgentID:          agentID,
		ActiveTaskID:     activeTaskID,
		Status:           model.SessionStatusActive,
		LastCheckpointAt: now,
		StartedAt:        now,
	}
	return session, nil
}

func (s *Store) GetActiveSession(ctx context.Context) (*model.Session, error) {
	query := `
		SELECT id, agent_id, active_task_id, status, checkpoint, handoff, summary, next_action, last_checkpoint_at, started_at, ended_at
		FROM sessions
		WHERE status = 'active'
		ORDER BY last_checkpoint_at DESC LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query)
	return scanSession(row)
}

func (s *Store) GetLastSession(ctx context.Context) (*model.Session, error) {
	query := `
		SELECT id, agent_id, active_task_id, status, checkpoint, handoff, summary, next_action, last_checkpoint_at, started_at, ended_at
		FROM sessions
		ORDER BY started_at DESC LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query)
	return scanSession(row)
}

func (s *Store) ListSessions(ctx context.Context, limit int) ([]*model.Session, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, agent_id, active_task_id, status, checkpoint, handoff, summary, next_action, last_checkpoint_at, started_at, ended_at
		FROM sessions
		ORDER BY started_at DESC
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*model.Session
	for rows.Next() {
		sess, err := scanSessionRowScanner(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading sessions: %w", err)
	}
	return sessions, nil
}

func (s *Store) GetSession(ctx context.Context, idOrPrefix string) (*model.Session, error) {
	idOrPrefix = strings.TrimSpace(idOrPrefix)
	if idOrPrefix == "" {
		return nil, ErrNotFound
	}
	query := `
		SELECT id, agent_id, active_task_id, status, checkpoint, handoff, summary, next_action, last_checkpoint_at, started_at, ended_at
		FROM sessions
		WHERE id = ? OR id LIKE ?
		ORDER BY started_at DESC LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query, idOrPrefix, idOrPrefix+"%")
	return scanSession(row)
}

func (s *Store) UpdateCheckpoint(ctx context.Context, sessionID string, activeTaskID *int64, note string) error {
	now := time.Now().UTC()
	query := `
		UPDATE sessions
		SET active_task_id = COALESCE(?, active_task_id),
		    checkpoint = CASE WHEN ? != '' THEN ? ELSE checkpoint END,
		    summary = CASE WHEN ? != '' THEN ? ELSE summary END,
		    last_checkpoint_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query,
		activeTaskID,
		note,
		note,
		note,
		note,
		now.Format(time.RFC3339),
		sessionID,
	)
	return err
}

func (s *Store) MarkInterrupted(ctx context.Context, sessionID string) error {
	now := time.Now().UTC()
	query := `
		UPDATE sessions
		SET status = 'interrupted', ended_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query, now.Format(time.RFC3339), sessionID)
	return err
}

func (s *Store) CloseSession(ctx context.Context, sessionID string, handoff string, legacyNextAction ...string) error {
	now := time.Now().UTC()
	var nextAction string
	if len(legacyNextAction) > 0 {
		nextAction = legacyNextAction[0]
	}
	query := `
		UPDATE sessions
		SET status = 'completed',
		    handoff = ?,
		    summary = ?,
		    next_action = CASE WHEN ? != '' THEN ? ELSE next_action END,
		    last_checkpoint_at = ?,
		    ended_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query,
		handoff,
		handoff,
		nextAction,
		nextAction,
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("failed to close session: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row *sql.Row) (*model.Session, error) {
	return scanSessionRowScanner(row)
}

func scanSessionRowScanner(s rowScanner) (*model.Session, error) {
	var session model.Session
	var activeTaskID sql.NullInt64
	var checkpoint, handoff, summary, nextAction sql.NullString
	var startedAtRaw, lastCheckpointRaw string
	var endedAtRaw sql.NullString

	err := s.Scan(
		&session.ID,
		&session.AgentID,
		&activeTaskID,
		&session.Status,
		&checkpoint,
		&handoff,
		&summary,
		&nextAction,
		&lastCheckpointRaw,
		&startedAtRaw,
		&endedAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan session error: %w", err)
	}

	if activeTaskID.Valid {
		tid := activeTaskID.Int64
		session.ActiveTaskID = &tid
	}
	session.Checkpoint = checkpoint.String
	session.Summary = summary.String
	session.NextAction = nextAction.String

	rawHandoff := handoff.String
	if rawHandoff == "" && summary.String != "" {
		rawHandoff = summary.String
	}
	session.Handoff = parseHandoff(rawHandoff)

	// Sync summary/nextAction backward-compatible fields if handoff is structured map
	if m, ok := session.Handoff.(map[string]any); ok {
		if session.Summary == "" {
			if sm, has := m["summary"]; has {
				session.Summary = fmt.Sprint(sm)
			}
		}
		if session.NextAction == "" {
			if na, has := m["next_action"]; has {
				session.NextAction = fmt.Sprint(na)
			} else if na, has := m["next"]; has {
				session.NextAction = fmt.Sprint(na)
			}
		}
	} else if session.Summary == "" && rawHandoff != "" {
		session.Summary = rawHandoff
	}

	session.LastCheckpointAt = parseTime(lastCheckpointRaw)
	session.StartedAt = parseTime(startedAtRaw)
	if endedAtRaw.Valid {
		t := parseTime(endedAtRaw.String)
		session.EndedAt = &t
	}
	return &session, nil
}

func parseHandoff(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if (strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}")) ||
		(strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]")) {
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return parsed
		}
	}
	return raw
}
