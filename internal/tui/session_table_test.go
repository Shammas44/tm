package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
	"taskmanager/internal/db"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

func TestSessionSorting(t *testing.T) {
	now := time.Now()
	ended1 := now.Add(10 * time.Minute)
	ended2 := now.Add(30 * time.Minute)

	s1 := &model.Session{
		ID:        "sess-001",
		AgentID:   "gemini",
		Status:    model.SessionStatusCompleted,
		StartedAt: now.Add(-30 * time.Minute),
		EndedAt:   &ended1, // duration 40m
		Summary:   "Alpha summary",
	}
	s2 := &model.Session{
		ID:        "sess-002",
		AgentID:   "claude",
		Status:    model.SessionStatusActive,
		StartedAt: now.Add(-10 * time.Minute),
		EndedAt:   nil, // duration 10m
		Checkpoint: "Beta checkpoint",
	}
	s3 := &model.Session{
		ID:        "sess-003",
		AgentID:   "antigravity",
		Status:    model.SessionStatusInterrupted,
		StartedAt: now.Add(-60 * time.Minute),
		EndedAt:   &ended2, // duration 90m
		Handoff:   map[string]any{"summary": "Gamma handoff"},
	}

	sessions := []*model.Session{s1, s2, s3}
	obsCounts := map[string]int{"sess-001": 5, "sess-002": 1, "sess-003": 12}
	taskCounts := map[string]int{"sess-001": 2, "sess-002": 4, "sess-003": 1}

	// 1. Sort by Agent ascending
	SortSessions(sessions, SessionSortOrder{Field: SessionSortByAgent, Ascending: true}, obsCounts, taskCounts)
	if sessions[0].AgentID != "antigravity" || sessions[1].AgentID != "claude" || sessions[2].AgentID != "gemini" {
		t.Fatalf("expected antigravity, claude, gemini; got %v, %v, %v", sessions[0].AgentID, sessions[1].AgentID, sessions[2].AgentID)
	}

	// 2. Sort by Notes descending
	SortSessions(sessions, SessionSortOrder{Field: SessionSortByNotes, Ascending: false}, obsCounts, taskCounts)
	if sessions[0].ID != "sess-003" || sessions[1].ID != "sess-001" || sessions[2].ID != "sess-002" {
		t.Fatalf("expected sess-003 (12), sess-001 (5), sess-002 (1); got %v, %v, %v", sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}

	// 3. Sort by Tasks descending
	SortSessions(sessions, SessionSortOrder{Field: SessionSortByTasks, Ascending: false}, obsCounts, taskCounts)
	if sessions[0].ID != "sess-002" || sessions[1].ID != "sess-001" || sessions[2].ID != "sess-003" {
		t.Fatalf("expected sess-002 (4), sess-001 (2), sess-003 (1); got %v, %v, %v", sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}

	// 4. Sort by Duration descending
	SortSessions(sessions, SessionSortOrder{Field: SessionSortByDuration, Ascending: false}, obsCounts, taskCounts)
	if sessions[0].ID != "sess-003" || sessions[1].ID != "sess-001" || sessions[2].ID != "sess-002" {
		t.Fatalf("expected sess-003 (90m), sess-001 (40m), sess-002 (10m); got %v, %v, %v", sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}
}

func TestMatchesSessionFilter(t *testing.T) {
	s := &model.Session{
		ID:         "sess-abc-123",
		AgentID:    "claude-opus",
		Status:     model.SessionStatusCompleted,
		Checkpoint: "Refactored database queries",
		Handoff:    "Next step: add caching",
	}

	if !MatchesSessionFilter(s, "abc") {
		t.Errorf("expected match on ID")
	}
	if !MatchesSessionFilter(s, "opus") {
		t.Errorf("expected match on AgentID")
	}
	if !MatchesSessionFilter(s, "completed") {
		t.Errorf("expected match on status")
	}
	if !MatchesSessionFilter(s, "caching") {
		t.Errorf("expected match on handoff")
	}
	if !MatchesSessionFilter(s, "database") {
		t.Errorf("expected match on checkpoint")
	}
	if MatchesSessionFilter(s, "nonexistent-query") {
		t.Errorf("expected no match on nonexistent query")
	}
}

func TestFormatSessionYAML(t *testing.T) {
	now := time.Now()
	ended := now.Add(25 * time.Minute)
	taskID := int64(42)

	sess := &model.Session{
		ID:           "sess-test-777",
		AgentID:      "test-agent",
		Status:       model.SessionStatusCompleted,
		StartedAt:    now,
		EndedAt:      &ended,
		ActiveTaskID: &taskID,
		Checkpoint:   "Final checkpoint before wrap up",
		Handoff: map[string]any{
			"summary":     "Completed task 42 and added unit tests",
			"next_action": "Deploy to staging",
		},
	}

	activeTask := &model.Task{
		ID:       42,
		Title:    "Implement session table in TUI",
		Status:   model.StatusCompleted,
		Priority: model.PriorityHigh,
		Project:  "TM-Core",
	}

	obs := []*model.Observation{
		{
			ID:        1,
			SessionID: &sess.ID,
			TaskID:    &taskID,
			CreatedAt: now.Add(5 * time.Minute),
			Content:   "First observation logged",
		},
	}

	touchedTasks := []*model.Task{activeTask}

	yamlContent := FormatSessionYAML(sess, activeTask, obs, touchedTasks)
	if !strings.Contains(yamlContent, "READ-ONLY SESSION INSPECTION: sess-test-777") {
		t.Errorf("expected header comment with session ID")
	}

	// Verify valid YAML parsing
	var parsed map[string]any
	// Skip header comments
	body := yamlContent[strings.Index(yamlContent, "id: sess-test-777"):]
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated session YAML: %v", err)
	}

	if parsed["id"] != "sess-test-777" {
		t.Errorf("expected parsed id to be 'sess-test-777', got %v", parsed["id"])
	}
	if parsed["agent"] != "test-agent" {
		t.Errorf("expected agent 'test-agent', got %v", parsed["agent"])
	}
	if parsed["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", parsed["status"])
	}

	dur := FormatSessionDuration(sess)
	if dur != "25m 0s" {
		t.Errorf("expected duration '25m 0s', got %v", dur)
	}
}

func TestTUITableSwitching(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test-tui.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()
	st := store.New(database)

	m := New(st)
	if m.activeTable != TableTasks {
		t.Fatalf("expected initial table to be TableTasks, got %v", m.activeTable)
	}

	// Press 't' to switch to Sessions table
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	modelCast, ok := newM.(Model)
	if !ok {
		t.Fatalf("expected Model instance")
	}
	if modelCast.activeTable != TableSessions {
		t.Fatalf("expected activeTable to be TableSessions after pressing 't', got %v", modelCast.activeTable)
	}

	// Press 't' again to return to Tasks table
	newM2, _ := modelCast.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	modelCast2 := newM2.(Model)
	if modelCast2.activeTable != TableTasks {
		t.Fatalf("expected activeTable to be TableTasks after pressing 't' again, got %v", modelCast2.activeTable)
	}

	// Test command mode :sessions and :tasks
	newM3, _ := modelCast2.executeCommand("sessions")
	if newM3.activeTable != TableSessions {
		t.Fatalf("expected :sessions command to switch to TableSessions, got %v", newM3.activeTable)
	}

	newM4, _ := newM3.executeCommand("tasks")
	if newM4.activeTable != TableTasks {
		t.Fatalf("expected :tasks command to switch to TableTasks, got %v", newM4.activeTable)
	}
}
