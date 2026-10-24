package tui

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"taskmanager/internal/model"
)

type SessionSortField int

const (
	SessionSortByID SessionSortField = iota
	SessionSortByStatus
	SessionSortByAgent
	SessionSortByStarted
	SessionSortByDuration
	SessionSortByTasks
	SessionSortByNotes
	SessionSortByHandoff
)

const TotalSessionColumns = 8

func ColumnToSessionSortField(colIdx int) SessionSortField {
	switch colIdx {
	case 0:
		return SessionSortByID
	case 1:
		return SessionSortByStatus
	case 2:
		return SessionSortByAgent
	case 3:
		return SessionSortByStarted
	case 4:
		return SessionSortByDuration
	case 5:
		return SessionSortByTasks
	case 6:
		return SessionSortByNotes
	case 7:
		return SessionSortByHandoff
	default:
		return SessionSortByStarted
	}
}

func DefaultSessionAscendingForField(field SessionSortField) bool {
	switch field {
	case SessionSortByStarted, SessionSortByDuration, SessionSortByTasks, SessionSortByNotes:
		return false // Newest, longest duration, or highest activity first by default
	default:
		return true // ID, Status, Agent, Handoff alphabetical A-Z
	}
}

type SessionSortOrder struct {
	Field     SessionSortField
	Ascending bool
}

func (so SessionSortOrder) Label() string {
	dir := "▲"
	if !so.Ascending {
		dir = "▼"
	}
	switch so.Field {
	case SessionSortByID:
		return "Session " + dir
	case SessionSortByStatus:
		return "Status " + dir
	case SessionSortByAgent:
		return "Agent " + dir
	case SessionSortByStarted:
		return "Started " + dir
	case SessionSortByDuration:
		return "Duration " + dir
	case SessionSortByTasks:
		return "Tasks " + dir
	case SessionSortByNotes:
		return "Notes " + dir
	case SessionSortByHandoff:
		return "Handoff " + dir
	default:
		return "Started " + dir
	}
}

func SortSessions(sessions []*model.Session, order SessionSortOrder, obsCounts map[string]int, taskCounts map[string]int) {
	sort.SliceStable(sessions, func(i, j int) bool {
		a, b := sessions[i], sessions[j]
		var less bool

		switch order.Field {
		case SessionSortByID:
			less = a.ID < b.ID
		case SessionSortByStatus:
			less = string(a.Status) < string(b.Status)
		case SessionSortByAgent:
			less = strings.ToLower(a.AgentID) < strings.ToLower(b.AgentID)
		case SessionSortByStarted:
			less = a.StartedAt.Before(b.StartedAt)
		case SessionSortByDuration:
			durA := sessionDurationVal(a)
			durB := sessionDurationVal(b)
			less = durA < durB
		case SessionSortByTasks:
			cntA := taskCounts[a.ID]
			cntB := taskCounts[b.ID]
			less = cntA < cntB
		case SessionSortByNotes:
			cntA := obsCounts[a.ID]
			cntB := obsCounts[b.ID]
			less = cntA < cntB
		case SessionSortByHandoff:
			strA := extractHandoffString(a)
			strB := extractHandoffString(b)
			less = strings.ToLower(strA) < strings.ToLower(strB)
		default:
			less = a.StartedAt.Before(b.StartedAt)
		}

		if order.Ascending {
			return less
		}
		return !less
	})
}

func sessionDurationVal(s *model.Session) time.Duration {
	if s.EndedAt != nil {
		return s.EndedAt.Sub(s.StartedAt)
	}
	return time.Since(s.StartedAt)
}

func extractHandoffString(s *model.Session) string {
	if s.Handoff != nil {
		if str, ok := s.Handoff.(string); ok && str != "" {
			return str
		}
		b, _ := json.Marshal(s.Handoff)
		return string(b)
	}
	if s.Checkpoint != "" {
		return s.Checkpoint
	}
	return s.Summary
}

func MatchesSessionFilter(s *model.Session, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}

	if strings.Contains(strings.ToLower(s.ID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.AgentID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(string(s.Status)), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Checkpoint), q) {
		return true
	}
	if strings.Contains(strings.ToLower(extractHandoffString(s)), q) {
		return true
	}
	return false
}
