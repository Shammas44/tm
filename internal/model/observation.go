package model

import "time"

type Observation struct {
	ID        int64     `json:"id"`
	SessionID *string   `json:"session_id,omitempty"`
	TaskID    *int64    `json:"task_id,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
