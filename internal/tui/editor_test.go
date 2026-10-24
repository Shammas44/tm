package tui

import (
	"strings"
	"testing"
	"time"

	"taskmanager/internal/model"
)

func TestParseNewTaskTemplate(t *testing.T) {
	input := `# TaskManager: Create New Task
# Lines starting with '#' are comments and will be ignored.
# Save and close the editor to create the task.

Title: Refactor compiler AST parser
Priority: H
Project: compiler
Tags: backend, parser, ast
Depends: 1, 2
Blocker Reason: 
New Note: Initial discovery on parser architecture

# Everything below 'Description:' will be treated as the task description.
Description:
Support ternary operators
and nested blocks.
`

	parsed, err := ParseTaskTemplate(input)
	if err != nil {
		t.Fatalf("ParseTaskTemplate failed: %v", err)
	}

	if parsed.Title != "Refactor compiler AST parser" {
		t.Errorf("expected title 'Refactor compiler AST parser', got '%s'", parsed.Title)
	}
	if parsed.Priority != model.PriorityHigh {
		t.Errorf("expected priority H, got %s", parsed.Priority)
	}
	if parsed.Project != "compiler" {
		t.Errorf("expected project 'compiler', got '%s'", parsed.Project)
	}
	if len(parsed.Tags) != 3 || parsed.Tags[0] != "backend" || parsed.Tags[1] != "parser" || parsed.Tags[2] != "ast" {
		t.Errorf("unexpected tags: %+v", parsed.Tags)
	}
	if len(parsed.Dependencies) != 2 || parsed.Dependencies[0] != 1 || parsed.Dependencies[1] != 2 {
		t.Errorf("unexpected dependencies: %+v", parsed.Dependencies)
	}
	if parsed.NewNote != "Initial discovery on parser architecture" {
		t.Errorf("expected new note, got '%s'", parsed.NewNote)
	}
	if parsed.Description != "Support ternary operators\nand nested blocks." {
		t.Errorf("expected multiline description, got '%s'", parsed.Description)
	}
}

func TestParseEditTaskTemplateWithObservations(t *testing.T) {
	task := &model.Task{
		ID:            42,
		Title:         "Fix memory leak in pool",
		Status:        model.StatusInProgress,
		Priority:      model.PriorityCritical,
		Project:       "engine",
		BlockerReason: "Waiting for valgrind run",
		Tags:          []string{"mem", "leak"},
		Dependencies:  []int64{10},
		Description:   "Reproduced with 100k iterations.\nNeeds custom slab allocator.",
		CreatedAt:     time.Now().Add(-2 * time.Hour),
		UpdatedAt:     time.Now().Add(-1 * time.Hour),
	}

	obs := []*model.Observation{
		{ID: 1, TaskID: &task.ID, Content: "Valgrind detected 48 bytes unfree", CreatedAt: time.Now()},
		{ID: 2, TaskID: &task.ID, Content: "Root cause found in pool_release()", CreatedAt: time.Now()},
	}

	allTasks := []*model.Task{task}

	tmpl := FormatEditTaskTemplate(task, allTasks, obs)
	if !strings.Contains(tmpl, "Note 1: Valgrind detected 48 bytes unfree") {
		t.Errorf("expected Note 1 in template")
	}
	if !strings.Contains(tmpl, "Note 2: Root cause found in pool_release()") {
		t.Errorf("expected Note 2 in template")
	}

	// Modify Note 1 and add New Note
	modifiedInput := strings.Replace(tmpl, "Note 1: Valgrind detected 48 bytes unfree", "Note 1: Valgrind detected 64 bytes unfree in slab_alloc", 1)
	modifiedInput = strings.Replace(modifiedInput, "New Note: \n", "New Note: Verified with ASan run on Linux\n", 1)

	parsed, err := ParseTaskTemplate(modifiedInput)
	if err != nil {
		t.Fatalf("ParseTaskTemplate failed: %v", err)
	}

	if parsed.Title != task.Title {
		t.Errorf("expected title '%s', got '%s'", task.Title, parsed.Title)
	}
	if parsed.ExistingNotes[1] != "Valgrind detected 64 bytes unfree in slab_alloc" {
		t.Errorf("expected modified note 1, got '%s'", parsed.ExistingNotes[1])
	}
	if parsed.ExistingNotes[2] != "Root cause found in pool_release()" {
		t.Errorf("expected unchanged note 2, got '%s'", parsed.ExistingNotes[2])
	}
	if parsed.NewNote != "Verified with ASan run on Linux" {
		t.Errorf("expected new note, got '%s'", parsed.NewNote)
	}
}
