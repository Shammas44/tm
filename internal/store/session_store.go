package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
		INSERT INTO sessions (id, agent_id, active_task_id, status, summary, next_action, last_checkpoint_at, started_at, ended_at)
		VALUES (?, ?, ?, ?, '', '', ?, ?, NULL)
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
		SELECT id, agent_id, active_task_id, status, summary, next_action, last_checkpoint_at, started_at, ended_at
		FROM sessions
		WHERE status = 'active'
		ORDER BY last_checkpoint_at DESC LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query)
	return scanSession(row)
}

func (s *Store) GetLastSession(ctx context.Context) (*model.Session, error) {
	query := `
		SELECT id, agent_id, active_task_id, status, summary, next_action, last_checkpoint_at, started_at, ended_at
		FROM sessions
		ORDER BY started_at DESC LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query)
	return scanSession(row)
}

func (s *Store) UpdateCheckpoint(ctx context.Context, sessionID string, activeTaskID *int64, note string) error {
	now := time.Now().UTC()
	query := `
		UPDATE sessions
		SET active_task_id = COALESCE(?, active_task_id),
		    summary = CASE WHEN ? != '' THEN ? ELSE summary END,
		    last_checkpoint_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query,
		activeTaskID,
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

func (s *Store) CloseSession(ctx context.Context, sessionID string, summary string, nextAction string) error {
	now := time.Now().UTC()
	query := `
		UPDATE sessions
		SET status = 'completed', summary = ?, next_action = ?, last_checkpoint_at = ?, ended_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query,
		summary,
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

func scanSession(row *sql.Row) (*model.Session, error) {
	var session model.Session
	var activeTaskID sql.NullInt64
	var summary, nextAction sql.NullString
	var startedAtRaw, lastCheckpointRaw string
	var endedAtRaw sql.NullString

	err := row.Scan(
		&session.ID,
		&session.AgentID,
		&activeTaskID,
		&session.Status,
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
	session.Summary = summary.String
	session.NextAction = nextAction.String
	session.LastCheckpointAt = parseTime(lastCheckpointRaw)
	session.StartedAt = parseTime(startedAtRaw)
	if endedAtRaw.Valid {
		t := parseTime(endedAtRaw.String)
		session.EndedAt = &t
	}
	return &session, nil
}
