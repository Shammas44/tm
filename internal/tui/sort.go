package tui

import (
	"sort"
	"strings"
	"taskmanager/internal/model"
)

type SortField int

const (
	SortByID SortField = iota
	SortByStatus
	SortByPriority
	SortByProject
	SortByTags
	SortByDeps
	SortByNotes
	SortByCreated
	SortByTitle
)

const TotalColumns = 9

func ColumnToSortField(colIdx int) SortField {
	switch colIdx {
	case 0:
		return SortByID
	case 1:
		return SortByStatus
	case 2:
		return SortByPriority
	case 3:
		return SortByProject
	case 4:
		return SortByTags
	case 5:
		return SortByDeps
	case 6:
		return SortByNotes
	case 7:
		return SortByCreated
	case 8:
		return SortByTitle
	default:
		return SortByID
	}
}

func SortFieldToColumn(field SortField) int {
	switch field {
	case SortByID:
		return 0
	case SortByStatus:
		return 1
	case SortByPriority:
		return 2
	case SortByProject:
		return 3
	case SortByTags:
		return 4
	case SortByDeps:
		return 5
	case SortByNotes:
		return 6
	case SortByCreated:
		return 7
	case SortByTitle:
		return 8
	default:
		return 0
	}
}

func DefaultAscendingForField(field SortField) bool {
	switch field {
	case SortByPriority, SortByCreated, SortByNotes:
		return false // Newest, highest priority, or most notes first by default
	default:
		return true // ID 1..N, A..Z
	}
}

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
	case SortByStatus:
		return "Status " + dir
	case SortByPriority:
		return "Pri " + dir
	case SortByProject:
		return "Project " + dir
	case SortByTags:
		return "Tags " + dir
	case SortByDeps:
		return "Deps " + dir
	case SortByNotes:
		return "Notes " + dir
	case SortByCreated:
		return "Created " + dir
	case SortByTitle:
		return "Title " + dir
	default:
		return "ID " + dir
	}
}

func SortTasks(tasks []*model.Task, order SortOrder, optionalObsCounts ...map[int64]int) {
	var obsCounts map[int64]int
	if len(optionalObsCounts) > 0 {
		obsCounts = optionalObsCounts[0]
	}

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
		case SortByStatus:
			if a.Status != b.Status {
				less = a.Status < b.Status
			} else {
				less = a.ID < b.ID
			}
		case SortByPriority:
			pa, pb := priorityWeight(a.Priority), priorityWeight(b.Priority)
			if pa != pb {
				less = pa < pb
			} else {
				less = a.ID < b.ID
			}
		case SortByProject:
			if a.Project != b.Project {
				less = a.Project < b.Project
			} else {
				less = a.ID < b.ID
			}
		case SortByTags:
			tagA := strings.Join(a.Tags, ",")
			tagB := strings.Join(b.Tags, ",")
			if tagA != tagB {
				less = tagA < tagB
			} else {
				less = a.ID < b.ID
			}
		case SortByDeps:
			if len(a.Dependencies) != len(b.Dependencies) {
				less = len(a.Dependencies) < len(b.Dependencies)
			} else {
				less = a.ID < b.ID
			}
		case SortByNotes:
			cntA, cntB := 0, 0
			if obsCounts != nil {
				cntA = obsCounts[a.ID]
				cntB = obsCounts[b.ID]
			}
			if cntA != cntB {
				less = cntA < cntB
			} else {
				less = a.ID < b.ID
			}
		case SortByCreated:
			if !a.CreatedAt.Equal(b.CreatedAt) {
				less = a.CreatedAt.Before(b.CreatedAt)
			} else {
				less = a.ID < b.ID
			}
		case SortByTitle:
			ta := strings.ToLower(a.Title)
			tb := strings.ToLower(b.Title)
			if ta != tb {
				less = ta < tb
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
