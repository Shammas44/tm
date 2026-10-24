package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"taskmanager/internal/model"
)

type TaskFilter struct {
	Status         model.TaskStatus
	Priority       model.TaskPriority
	Project        string
	IncludeDeleted bool
}

type BacklogCounts struct {
	Pending    int `json:"pending"`
	Started    int `json:"started"`
	InProgress int `json:"in_progress,omitempty"`
	Blocked    int `json:"blocked"`
	Completed  int `json:"completed"`
}

func (s *Store) CreateTask(ctx context.Context, task *model.Task) (*model.Task, error) {
	if task.Status == "" {
		task.Status = model.StatusPending
	}
	if task.Priority == "" {
		task.Priority = model.PriorityMedium
	}
	if task.Tags == nil {
		task.Tags = []string{}
	}
	if task.Dependencies == nil {
		task.Dependencies = []int64{}
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	tagsJSON, _ := json.Marshal(task.Tags)
	depsJSON, _ := json.Marshal(task.Dependencies)

	query := `
		INSERT INTO tasks (title, description, status, priority, project, active, blocker, tags, dependencies, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	activeInt := 0
	if task.Active {
		activeInt = 1
	}

	res, err := s.db.ExecContext(ctx, query,
		task.Title,
		task.Description,
		string(task.Status),
		string(task.Priority),
		task.Project,
		activeInt,
		task.Blocker,
		string(tagsJSON),
		string(depsJSON),
		task.CreatedAt.Format(time.RFC3339),
		task.UpdatedAt.Format(time.RFC3339),
		formatNullableTime(task.CompletedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last insert id: %w", err)
	}
	task.ID = id
	return task, nil
}

func (s *Store) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	query := `
		SELECT id, title, description, status, priority, project, active, blocker, tags, dependencies, created_at, updated_at, completed_at
		FROM tasks WHERE id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)
	return scanTask(row)
}

func (s *Store) GetActiveTask(ctx context.Context, project string) (*model.Task, error) {
	query := `
		SELECT id, title, description, status, priority, project, active, blocker, tags, dependencies, created_at, updated_at, completed_at
		FROM tasks WHERE active = 1
	`
	var args []any
	if project != "" {
		query += ` AND project = ?`
		args = append(args, project)
	}
	query += ` ORDER BY updated_at DESC LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, args...)
	return scanTask(row)
}

func (s *Store) SetActiveTask(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	// Unset active on all other tasks
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET active = 0 WHERE active = 1 AND id != ?`, id)
	if err != nil {
		return fmt.Errorf("failed to unset existing active tasks: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET active = 1, status = ?, blocker = '', updated_at = ?
		WHERE id = ?
	`, string(model.StatusStarted), now.Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to set active task: %w", err)
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

func (s *Store) UpdateTaskStatus(ctx context.Context, id int64, status model.TaskStatus, blockerReason string) error {
	now := time.Now().UTC()
	activeInt := 0
	var completedAt any
	if status == model.StatusStarted {
		activeInt = 1
	} else if status == model.StatusCompleted {
		completedAt = now.Format(time.RFC3339)
	}

	if activeInt == 1 {
		_, _ = s.db.ExecContext(ctx, `UPDATE tasks SET active = 0 WHERE active = 1 AND id != ?`, id)
	}

	query := `
		UPDATE tasks
		SET status = ?, active = ?, blocker = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query,
		string(status),
		activeInt,
		blockerReason,
		now.Format(time.RFC3339),
		completedAt,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
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

func (s *Store) UpdateTask(ctx context.Context, task *model.Task) error {
	task.UpdatedAt = time.Now().UTC()
	if task.Status == model.StatusCompleted && task.CompletedAt == nil {
		now := task.UpdatedAt
		task.CompletedAt = &now
		task.Active = false
	} else if task.Status != model.StatusCompleted {
		task.CompletedAt = nil
	}

	if task.Tags == nil {
		task.Tags = []string{}
	}
	if task.Dependencies == nil {
		task.Dependencies = []int64{}
	}
	tagsJSON, _ := json.Marshal(task.Tags)
	depsJSON, _ := json.Marshal(task.Dependencies)

	activeInt := 0
	if task.Active {
		activeInt = 1
		_, _ = s.db.ExecContext(ctx, `UPDATE tasks SET active = 0 WHERE active = 1 AND id != ?`, task.ID)
	}

	query := `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, priority = ?, project = ?, active = ?, blocker = ?, tags = ?, dependencies = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query,
		task.Title,
		task.Description,
		string(task.Status),
		string(task.Priority),
		task.Project,
		activeInt,
		task.Blocker,
		string(tagsJSON),
		string(depsJSON),
		task.UpdatedAt.Format(time.RFC3339),
		formatNullableTime(task.CompletedAt),
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
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

func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	query := `UPDATE tasks SET status = 'cancelled', active = 0, updated_at = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
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

func (s *Store) DeleteTaskPermanently(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear active_task_id references in sessions
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET active_task_id = NULL WHERE active_task_id = ?`, id); err != nil {
		return fmt.Errorf("failed to clear session active task: %w", err)
	}

	// Delete observations (foreign key references task_id)
	// Triggers on observations automatically clean up observations_fts
	if _, err := tx.ExecContext(ctx, `DELETE FROM observations WHERE task_id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete task observations: %w", err)
	}

	// Delete the task record completely
	res, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to permanently delete task: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) ListTasks(ctx context.Context, filter TaskFilter) ([]*model.Task, error) {
	query := `SELECT id, title, description, status, priority, project, active, blocker, tags, dependencies, created_at, updated_at, completed_at FROM tasks WHERE 1=1`
	var args []any

	if !filter.IncludeDeleted {
		query += ` AND status != 'cancelled'`
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(filter.Status))
	}
	if filter.Priority != "" {
		query += ` AND priority = ?`
		args = append(args, string(filter.Priority))
	}
	if filter.Project != "" {
		query += ` AND project = ?`
		args = append(args, filter.Project)
	}

	query += ` ORDER BY id ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) GetBacklogCounts(ctx context.Context, project string) (BacklogCounts, error) {
	var counts BacklogCounts
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'started' OR status = 'in_progress' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0)
		FROM tasks
	`
	var args []any
	if project != "" {
		query += ` WHERE project = ?`
		args = append(args, project)
	}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&counts.Pending,
		&counts.Started,
		&counts.Blocked,
		&counts.Completed,
	)
	counts.InProgress = counts.Started
	return counts, err
}

func scanTask(row *sql.Row) (*model.Task, error) {
	var task model.Task
	var activeInt int
	var createdAtRaw, updatedAtRaw string
	var completedAtRaw sql.NullString
	var desc, blocker, project, tagsRaw, depsRaw sql.NullString

	err := row.Scan(
		&task.ID,
		&task.Title,
		&desc,
		&task.Status,
		&task.Priority,
		&project,
		&activeInt,
		&blocker,
		&tagsRaw,
		&depsRaw,
		&createdAtRaw,
		&updatedAtRaw,
		&completedAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan task error: %w", err)
	}

	task.Description = desc.String
	task.Project = project.String
	task.Active = activeInt == 1
	task.Blocker = blocker.String
	if tagsRaw.Valid && tagsRaw.String != "" {
		_ = json.Unmarshal([]byte(tagsRaw.String), &task.Tags)
	}
	if depsRaw.Valid && depsRaw.String != "" {
		_ = json.Unmarshal([]byte(depsRaw.String), &task.Dependencies)
	}
	if task.Tags == nil {
		task.Tags = []string{}
	}
	if task.Dependencies == nil {
		task.Dependencies = []int64{}
	}
	task.CreatedAt = parseTime(createdAtRaw)
	task.UpdatedAt = parseTime(updatedAtRaw)
	if completedAtRaw.Valid {
		t := parseTime(completedAtRaw.String)
		task.CompletedAt = &t
	}
	return &task, nil
}

func scanTaskRow(rows *sql.Rows) (*model.Task, error) {
	var task model.Task
	var activeInt int
	var createdAtRaw, updatedAtRaw string
	var completedAtRaw sql.NullString
	var desc, blocker, project, tagsRaw, depsRaw sql.NullString

	err := rows.Scan(
		&task.ID,
		&task.Title,
		&desc,
		&task.Status,
		&task.Priority,
		&project,
		&activeInt,
		&blocker,
		&tagsRaw,
		&depsRaw,
		&createdAtRaw,
		&updatedAtRaw,
		&completedAtRaw,
	)
	if err != nil {
		return nil, fmt.Errorf("scan task row error: %w", err)
	}

	task.Description = desc.String
	task.Project = project.String
	task.Active = activeInt == 1
	task.Blocker = blocker.String
	if tagsRaw.Valid && tagsRaw.String != "" {
		_ = json.Unmarshal([]byte(tagsRaw.String), &task.Tags)
	}
	if depsRaw.Valid && depsRaw.String != "" {
		_ = json.Unmarshal([]byte(depsRaw.String), &task.Dependencies)
	}
	if task.Tags == nil {
		task.Tags = []string{}
	}
	if task.Dependencies == nil {
		task.Dependencies = []int64{}
	}
	task.CreatedAt = parseTime(createdAtRaw)
	task.UpdatedAt = parseTime(updatedAtRaw)
	if completedAtRaw.Valid {
		t := parseTime(completedAtRaw.String)
		task.CompletedAt = &t
	}
	return &task, nil
}

func parseTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err == nil {
		return t
	}
	t, err = time.Parse("2006-01-02 15:04:05", raw)
	if err == nil {
		return t
	}
	return time.Time{}
}

func formatNullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}
