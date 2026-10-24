package tui

import (
	"sort"
	"taskmanager/internal/model"
)

type SortField int

const (
	SortByID SortField = iota
	SortByPriority
	SortByCreated
	SortByStatus
	SortByProject
	SortByTitle
)

type SortOrder struct {
	Field     SortField
	Ascending bool
}

func (so SortOrder) Label() string {
	dir := "▲"
	if !so.Ascending {
		dir = "▼"
	}
	switch so.Field {
	case SortByID:
		return "ID " + dir
	case SortByPriority:
		return "Pri " + dir
	case SortByCreated:
		return "Created " + dir
	case SortByStatus:
		return "Status " + dir
	case SortByProject:
		return "Project " + dir
	case SortByTitle:
		return "Title " + dir
	default:
		return "ID " + dir
	}
}

func (so *SortOrder) CycleNext() {
	switch so.Field {
	case SortByID:
		so.Field = SortByPriority
		so.Ascending = false // Highest priority first
	case SortByPriority:
		so.Field = SortByCreated
		so.Ascending = false // Newest first
	case SortByCreated:
		so.Field = SortByStatus
		so.Ascending = true
	case SortByStatus:
		so.Field = SortByProject
		so.Ascending = true
	case SortByProject:
		so.Field = SortByTitle
		so.Ascending = true
	case SortByTitle:
		so.Field = SortByID
		so.Ascending = true
	}
}

func SortTasks(tasks []*model.Task, order SortOrder) {
	priorityWeight := func(p model.TaskPriority) int {
		switch p {
		case model.PriorityCritical:
			return 4
		case model.PriorityHigh:
			return 3
		case model.PriorityMedium:
			return 2
		case model.PriorityLow:
			return 1
		default:
			return 0
		}
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		var less bool
		switch order.Field {
		case SortByID:
			less = a.ID < b.ID
		case SortByPriority:
			pa, pb := priorityWeight(a.Priority), priorityWeight(b.Priority)
			if pa != pb {
				less = pa < pb
			} else {
				less = a.ID < b.ID
			}
		case SortByCreated:
			if !a.CreatedAt.Equal(b.CreatedAt) {
				less = a.CreatedAt.Before(b.CreatedAt)
			} else {
				less = a.ID < b.ID
			}
		case SortByStatus:
			if a.Status != b.Status {
				less = a.Status < b.Status
			} else {
				less = a.ID < b.ID
			}
		case SortByProject:
			if a.Project != b.Project {
				less = a.Project < b.Project
			} else {
				less = a.ID < b.ID
			}
		case SortByTitle:
			if a.Title != b.Title {
				less = a.Title < b.Title
			} else {
				less = a.ID < b.ID
			}
		default:
			less = a.ID < b.ID
		}

		if !order.Ascending {
			return !less
		}
		return less
	})
}
