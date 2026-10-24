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
	if started.Status != model.StatusInProgress || !started.Active {
		t.Errorf("expected in_progress status and active=true, got: %+v", started)
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
	if blocked.Status != model.StatusBlocked || blocked.Active || blocked.BlockerReason != "Need code review approval" {
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
}
