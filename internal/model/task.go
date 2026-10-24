package model

import "time"

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusCompleted  TaskStatus = "completed"
	StatusBlocked    TaskStatus = "blocked"
	StatusCancelled  TaskStatus = "cancelled"
)

type TaskPriority string

const (
	PriorityLow      TaskPriority = "L"
	PriorityMedium   TaskPriority = "M"
	PriorityHigh     TaskPriority = "H"
	PriorityCritical TaskPriority = "C"
)

type Task struct {
	ID            int64        `json:"id"`
	Title         string       `json:"title"`
	Description   string       `json:"description,omitempty"`
	Status        TaskStatus   `json:"status"`
	Priority      TaskPriority `json:"priority"`
	Project       string       `json:"project,omitempty"`
	Active        bool         `json:"active"`
	BlockerReason string       `json:"blocker_reason,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	CompletedAt   *time.Time   `json:"completed_at,omitempty"`
}
