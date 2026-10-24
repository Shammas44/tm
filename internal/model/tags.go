package model

import (
	"time"
)

// ComputeVirtualTags calculates Taskwarrior-style synthetic tags based on task state, dependencies, and backlog context.
func ComputeVirtualTags(t *Task, allTasks []*Task, hasObservations bool) []string {
	var vtags []string

	// Status virtual tags
	if t.Active || t.Status == StatusStarted {
		vtags = append(vtags, "ACTIVE")
	}
	if t.Status == StatusPending {
		vtags = append(vtags, "PENDING")
	}
	if t.Status == StatusCompleted {
		vtags = append(vtags, "COMPLETED")
	}
	if t.Status == StatusCancelled {
		vtags = append(vtags, "CANCELLED")
	}

	// Priority virtual tags
	switch t.Priority {
	case PriorityCritical:
		vtags = append(vtags, "PRI_CRIT")
	case PriorityHigh:
		vtags = append(vtags, "PRI_HIGH")
	case PriorityMedium:
		vtags = append(vtags, "PRI_MED")
	case PriorityLow:
		vtags = append(vtags, "PRI_LOW")
	}

	// Dependency & Blocker virtual tags
	isBlocked := t.Status == StatusBlocked || t.Blocker != ""
	if !isBlocked && len(t.Dependencies) > 0 {
		// Check if any prerequisite is incomplete
		completedMap := make(map[int64]bool)
		for _, other := range allTasks {
			if other.Status == StatusCompleted {
				completedMap[other.ID] = true
			}
		}
		for _, depID := range t.Dependencies {
			if !completedMap[depID] {
				isBlocked = true
				break
			}
		}
	}

	if isBlocked {
		vtags = append(vtags, "BLOCKED")
	} else {
		vtags = append(vtags, "UNBLOCKED")
	}

	// READY tag: pending and not blocked
	if t.Status == StatusPending && !isBlocked {
		vtags = append(vtags, "READY")
	}

	// BLOCKING tag: another task depends on this task and this task is not completed
	if t.Status != StatusCompleted {
		for _, other := range allTasks {
			if other.Status != StatusCompleted {
				for _, depID := range other.Dependencies {
					if depID == t.ID {
						vtags = append(vtags, "BLOCKING")
						goto blockingChecked
					}
				}
			}
		}
	}
blockingChecked:

	// Note virtual tag
	if hasObservations || t.Description != "" {
		vtags = append(vtags, "HAS_NOTE")
	}

	// Age tag
	now := time.Now().UTC()
	if t.CreatedAt.Year() == now.Year() && t.CreatedAt.YearDay() == now.YearDay() {
		vtags = append(vtags, "TODAY")
	}

	return vtags
}

// AllTags returns user tags combined with uppercase virtual tags (e.g. +backend, +READY)
func (t *Task) AllTags(allTasks []*Task, hasObservations bool) []string {
	combined := append([]string{}, t.Tags...)
	vtags := ComputeVirtualTags(t, allTasks, hasObservations)
	combined = append(combined, vtags...)
	return combined
}
