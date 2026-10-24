package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"taskmanager/internal/model"
)

func TestPreprocessArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "positional start",
			input:    []string{"1", "start"},
			expected: []string{"start", "1"},
		},
		{
			name:     "positional done",
			input:    []string{"42", "done"},
			expected: []string{"done", "42"},
		},
		{
			name:     "positional block with reason",
			input:    []string{"5", "block", "waiting", "for", "review"},
			expected: []string{"block", "5", "waiting", "for", "review"},
		},
		{
			name:     "positional log with comment",
			input:    []string{"7", "log", "investigating bug"},
			expected: []string{"log", "7", "investigating bug"},
		},
		{
			name:     "positional show",
			input:    []string{"3"},
			expected: []string{"show", "3"},
		},
		{
			name:     "positional edit",
			input:    []string{"1", "edit"},
			expected: []string{"edit", "1"},
		},
		{
			name:     "positional tag",
			input:    []string{"1", "tag", "+cli", "-old"},
			expected: []string{"tag", "1", "+cli", "-old"},
		},
		{
			name:     "positional dep",
			input:    []string{"1", "dep", "5"},
			expected: []string{"dep", "1", "5"},
		},
		{
			name:     "with flags and positional start",
			input:    []string{"--json", "2", "start"},
			expected: []string{"--json", "start", "2"},
		},
		{
			name:     "with db flag taking value",
			input:    []string{"--db", "/path/to/db", "2", "done"},
			expected: []string{"--db", "/path/to/db", "done", "2"},
		},
		{
			name:     "standard non-positional syntax unchanged",
			input:    []string{"add", "New task", "priority:H"},
			expected: []string{"add", "New task", "priority:H"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := PreprocessArgs(tt.input)
			if !reflect.DeepEqual(res, tt.expected) {
				t.Errorf("PreprocessArgs(%v) = %v, expected %v", tt.input, res, tt.expected)
			}
		})
	}
}

func TestCLICommandsEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	testDBPath := filepath.Join(tempDir, "state.db")

	runCmd := func(args ...string) (string, error) {
		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)

		fullArgs := append([]string{"--db", testDBPath, "--json"}, args...)
		processed := PreprocessArgs(fullArgs)
		RootCmd.SetArgs(processed)
		err := RootCmd.Execute()
		return buf.String(), err
	}

	// 1. Add Task
	out, err := runCmd("add", "Write CLI integration tests", "priority:H", "project:calc")
	if err != nil {
		t.Fatalf("tm add failed: %v, out: %s", err, out)
	}

	var created model.Task
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("failed to parse JSON from add output: %v (raw: %s)", err, out)
	}
	if created.ID != 1 || created.Priority != model.PriorityHigh || created.Project != "calc" {
		t.Errorf("unexpected created task: %+v", created)
	}

	// 2. Start Task (positional)
	out, err = runCmd("1", "start")
	if err != nil {
		t.Fatalf("tm 1 start failed: %v, out: %s", err, out)
	}
	var started model.Task
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatalf("failed to parse JSON from start output: %v (raw: %s)", err, out)
	}
	if started.Status != model.StatusStarted || !started.Active {
		t.Errorf("expected started status and active=true, got: %+v", started)
	}

	// 3. Log Observation
	out, err = runCmd("1", "log", "Verified Cobra args reordering and FTS5 triggers")
	if err != nil {
		t.Fatalf("tm 1 log failed: %v, out: %s", err, out)
	}
	var obs model.Observation
	if err := json.Unmarshal([]byte(out), &obs); err != nil {
		t.Fatalf("failed to parse JSON from log output: %v (raw: %s)", err, out)
	}
	if obs.TaskID == nil || *obs.TaskID != 1 {
		t.Errorf("expected TaskID 1 on observation, got %v", obs.TaskID)
	}

	// 4. Search Observation via FTS5
	out, err = runCmd("search", "reordering")
	if err != nil {
		t.Fatalf("tm search failed: %v, out: %s", err, out)
	}
	var searchResults []model.Observation
	if err := json.Unmarshal([]byte(out), &searchResults); err != nil {
		t.Fatalf("failed to parse JSON from search output: %v (raw: %s)", err, out)
	}
	if len(searchResults) != 1 || searchResults[0].ID != obs.ID {
		t.Errorf("expected 1 search result matching 'reordering', got %d", len(searchResults))
	}

	// 5. Block Task
	out, err = runCmd("1", "block", "Need code review approval")
	if err != nil {
		t.Fatalf("tm 1 block failed: %v, out: %s", err, out)
	}
	var blocked model.Task
	if err := json.Unmarshal([]byte(out), &blocked); err != nil {
		t.Fatalf("failed to parse JSON from block output: %v (raw: %s)", err, out)
	}
	if blocked.Status != model.StatusBlocked || blocked.Active || blocked.Blocker != "Need code review approval" {
		t.Errorf("unexpected blocked task: %+v", blocked)
	}

	// 6. Complete Task
	out, err = runCmd("1", "done")
	if err != nil {
		t.Fatalf("tm 1 done failed: %v, out: %s", err, out)
	}
	var done model.Task
	if err := json.Unmarshal([]byte(out), &done); err != nil {
		t.Fatalf("failed to parse JSON from done output: %v (raw: %s)", err, out)
	}
	if done.Status != model.StatusCompleted || done.Active {
		t.Errorf("expected completed status, got %s", done.Status)
	}

	// 7. List Tasks
	out, err = runCmd("list", "--all")
	if err != nil {
		t.Fatalf("tm list failed: %v, out: %s", err, out)
	}
	var listResults []*model.Task
	if err := json.Unmarshal([]byte(out), &listResults); err != nil {
		t.Fatalf("failed to parse JSON from list output: %v (raw: %s)", err, out)
	}
	if len(listResults) != 1 || listResults[0].ID != 1 {
		t.Errorf("expected 1 task in list, got %d", len(listResults))
	}

	// 8. Edit Task via Flags
	out, err = runCmd("edit", "1", "--title", "Updated task title", "--priority", "C", "--tags", "go,cli", "--depends", "2,3")
	if err != nil {
		t.Fatalf("tm edit failed: %v, out: %s", err, out)
	}
	var edited model.Task
	if err := json.Unmarshal([]byte(out), &edited); err != nil {
		t.Fatalf("failed to parse JSON from edit output: %v (raw: %s)", err, out)
	}
	if edited.Title != "Updated task title" || edited.Priority != model.PriorityCritical {
		t.Errorf("expected edited task title and priority C, got %+v", edited)
	}
	if len(edited.Tags) != 2 || len(edited.Dependencies) != 2 {
		t.Errorf("expected 2 tags and 2 dependencies, got tags=%v deps=%v", edited.Tags, edited.Dependencies)
	}

	// 9. Tag manipulation (+tag, -tag)
	out, err = runCmd("tag", "1", "+backend", "-go")
	if err != nil {
		t.Fatalf("tm tag failed: %v, out: %s", err, out)
	}
	var tagged model.Task
	if err := json.Unmarshal([]byte(out), &tagged); err != nil {
		t.Fatalf("failed to parse JSON from tag output: %v (raw: %s)", err, out)
	}
	tagSet := make(map[string]bool)
	for _, tg := range tagged.Tags {
		tagSet[tg] = true
	}
	if !tagSet["backend"] || !tagSet["cli"] || tagSet["go"] {
		t.Errorf("expected tags [backend, cli] without 'go', got: %v", tagged.Tags)
	}

	// 10. Dependency manipulation (+dep, -dep)
	out, err = runCmd("dep", "1", "+4", "-2")
	if err != nil {
		t.Fatalf("tm dep failed: %v, out: %s", err, out)
	}
	var depTask model.Task
	if err := json.Unmarshal([]byte(out), &depTask); err != nil {
		t.Fatalf("failed to parse JSON from dep output: %v (raw: %s)", err, out)
	}
	depSet := make(map[int64]bool)
	for _, d := range depTask.Dependencies {
		depSet[d] = true
	}
	if !depSet[4] || !depSet[3] || depSet[2] {
		t.Errorf("expected dependencies [3, 4] without 2, got: %v", depTask.Dependencies)
	}

	// 11. Show Task Details
	out, err = runCmd("show", "1")
	if err != nil {
		t.Fatalf("tm show failed: %v, out: %s", err, out)
	}
	var richTask struct {
		model.Task
		Observations []*model.Observation `json:"observations"`
	}
	if err := json.Unmarshal([]byte(out), &richTask); err != nil {
		t.Fatalf("failed to parse JSON from show output: %v (raw: %s)", err, out)
	}
	if richTask.ID != 1 || len(richTask.Observations) == 0 {
		t.Errorf("expected task #1 with observations, got %+v", richTask)
	}

	// 12. Query Project Status (read-only without initiating a session)
	out, err = runCmd("status")
	if err != nil {
		t.Fatalf("tm status failed: %v, out: %s", err, out)
	}
	var statusRes StatusResponse
	if err := json.Unmarshal([]byte(out), &statusRes); err != nil {
		t.Fatalf("failed to parse JSON from status output: %v (raw: %s)", err, out)
	}
	if statusRes.Backlog.Completed != 1 {
		t.Errorf("expected 1 completed task in backlog, got %+v", statusRes.Backlog)
	}
	if len(statusRes.RecentObservations) == 0 {
		t.Errorf("expected recent observations in status, got %+v", statusRes.RecentObservations)
	}
}
