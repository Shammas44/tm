package model

import "time"

type SessionStatus string

const (
	SessionStatusActive      SessionStatus = "active"
	SessionStatusCompleted   SessionStatus = "completed"
	SessionStatusInterrupted SessionStatus = "interrupted"
)

type Session struct {
	ID               string        `json:"id"`
	AgentID          string        `json:"agent_id"`
	ActiveTaskID     *int64        `json:"active_task_id,omitempty"`
	Status           SessionStatus `json:"status"`
	Checkpoint       string        `json:"checkpoint,omitempty"`
	Handoff          any           `json:"handoff,omitempty"`
	Summary          string        `json:"summary,omitempty"`     // backward-compatible alias
	NextAction       string        `json:"next_action,omitempty"` // backward-compatible alias
	LastCheckpointAt time.Time     `json:"last_checkpoint_at"`
	StartedAt        time.Time     `json:"started_at"`
	EndedAt          *time.Time    `json:"ended_at,omitempty"`
}
