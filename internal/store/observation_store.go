package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"taskmanager/internal/model"
)

type ObservationFilter struct {
	SessionID string
	TaskID    *int64
	Limit     int
}

func (s *Store) AddObservation(ctx context.Context, sessionID *string, taskID *int64, content string) (*model.Observation, error) {
	now := time.Now().UTC()
	query := `
		INSERT INTO observations (session_id, task_id, content, created_at)
		VALUES (?, ?, ?, ?)
	`
	res, err := s.db.ExecContext(ctx, query,
		sessionID,
		taskID,
		content,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert observation: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last insert id: %w", err)
	}

	obs := &model.Observation{
		ID:        id,
		SessionID: sessionID,
		TaskID:    taskID,
		Content:   content,
		CreatedAt: now,
	}
	return obs, nil
}

func (s *Store) GetObservation(ctx context.Context, id int64) (*model.Observation, error) {
	query := `
		SELECT id, session_id, task_id, content, created_at
		FROM observations WHERE id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)
	return scanObservation(row)
}

func (s *Store) UpdateObservation(ctx context.Context, id int64, content string) error {
	query := `UPDATE observations SET content = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, content, id)
	if err != nil {
		return fmt.Errorf("failed to update observation #%d: %w", id, err)
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

func (s *Store) DeleteObservation(ctx context.Context, id int64) error {
	query := `DELETE FROM observations WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete observation #%d: %w", id, err)
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

func (s *Store) ListObservations(ctx context.Context, filter ObservationFilter) ([]*model.Observation, error) {
	query := `
		SELECT id, session_id, task_id, content, created_at
		FROM observations WHERE 1=1
	`
	var args []any

	if filter.SessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, filter.SessionID)
	}
	if filter.TaskID != nil {
		query += ` AND task_id = ?`
		args = append(args, *filter.TaskID)
	}

	query += ` ORDER BY id DESC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list observations: %w", err)
	}
	defer rows.Close()

	var results []*model.Observation
	for rows.Next() {
		obs, err := scanObservationRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, obs)
	}
	return results, rows.Err()
}

func (s *Store) SearchObservations(ctx context.Context, queryStr string) ([]*model.Observation, error) {
	queryStr = strings.TrimSpace(queryStr)
	if queryStr == "" {
		return s.ListObservations(ctx, ObservationFilter{Limit: 20})
	}

	sanitizedQuery := escapeFTS5Query(queryStr)

	query := `
		SELECT o.id, o.session_id, o.task_id, o.content, o.created_at
		FROM observations_fts fts
		JOIN observations o ON o.id = fts.rowid
		WHERE observations_fts MATCH ?
		ORDER BY rank
	`
	rows, err := s.db.QueryContext(ctx, query, sanitizedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to search observations: %w", err)
	}
	defer rows.Close()

	var results []*model.Observation
	for rows.Next() {
		obs, err := scanObservationRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, obs)
	}
	return results, rows.Err()
}

func scanObservation(row *sql.Row) (*model.Observation, error) {
	var obs model.Observation
	var sessionID sql.NullString
	var taskID sql.NullInt64
	var createdAtRaw string

	err := row.Scan(
		&obs.ID,
		&sessionID,
		&taskID,
		&obs.Content,
		&createdAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan observation error: %w", err)
	}

	if sessionID.Valid {
		s := sessionID.String
		obs.SessionID = &s
	}
	if taskID.Valid {
		tid := taskID.Int64
		obs.TaskID = &tid
	}
	obs.CreatedAt = parseTime(createdAtRaw)
	return &obs, nil
}

func scanObservationRow(rows *sql.Rows) (*model.Observation, error) {
	var obs model.Observation
	var sessionID sql.NullString
	var taskID sql.NullInt64
	var createdAtRaw string

	err := rows.Scan(
		&obs.ID,
		&sessionID,
		&taskID,
		&obs.Content,
		&createdAtRaw,
	)
	if err != nil {
		return nil, fmt.Errorf("scan observation row error: %w", err)
	}

	if sessionID.Valid {
		s := sessionID.String
		obs.SessionID = &s
	}
	if taskID.Valid {
		tid := taskID.Int64
		obs.TaskID = &tid
	}
	obs.CreatedAt = parseTime(createdAtRaw)
	return &obs, nil
}

func (s *Store) GetObservationCounts(ctx context.Context) (map[int64]int, error) {
	query := `
		SELECT task_id, COUNT(*)
		FROM observations
		WHERE task_id IS NOT NULL
		GROUP BY task_id
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query observation counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[int64]int)
	for rows.Next() {
		var taskID int64
		var count int
		if err := rows.Scan(&taskID, &count); err != nil {
			return nil, fmt.Errorf("failed to scan observation count: %w", err)
		}
		counts[taskID] = count
	}
	return counts, rows.Err()
}

func escapeFTS5Query(q string) string {
	q = strings.ReplaceAll(q, `"`, `""`)
	words := strings.Fields(q)
	var quoted []string
	for _, w := range words {
		quoted = append(quoted, `"`+w+`"*`)
	}
	return strings.Join(quoted, " ")
}
