package tui

import (
	"testing"

	"taskmanager/internal/model"
)

func TestParseNewTaskTemplate(t *testing.T) {
	input := `# TaskManager: Create New Task
# Lines starting with '#' are comments and will be ignored.
# Save and close the editor to create the task.

Title: Refactor compiler AST parser
Priority: H
Project: compiler
Blocker Reason: 

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
	if parsed.Status != model.StatusPending {
		t.Errorf("expected default pending status, got %s", parsed.Status)
	}
	if parsed.Description != "Support ternary operators\nand nested blocks." {
		t.Errorf("expected multiline description, got '%s'", parsed.Description)
	}
}

func TestParseEditTaskTemplate(t *testing.T) {
	task := &model.Task{
		ID:            42,
		Title:         "Fix memory leak in pool",
		Status:        model.StatusInProgress,
		Priority:      model.PriorityCritical,
		Project:       "engine",
		BlockerReason: "Waiting for valgrind run",
		Description:   "Reproduced with 100k iterations.\nNeeds custom slab allocator.",
	}

	tmpl := FormatEditTaskTemplate(task)
	parsed, err := ParseTaskTemplate(tmpl)
	if err != nil {
		t.Fatalf("ParseTaskTemplate failed: %v", err)
	}

	if parsed.Title != task.Title {
		t.Errorf("expected title '%s', got '%s'", task.Title, parsed.Title)
	}
	if parsed.Status != model.StatusInProgress {
		t.Errorf("expected status '%s', got '%s'", task.Status, parsed.Status)
	}
	if parsed.Priority != model.PriorityCritical {
		t.Errorf("expected priority '%s', got '%s'", task.Priority, parsed.Priority)
	}
	if parsed.Project != task.Project {
		t.Errorf("expected project '%s', got '%s'", task.Project, parsed.Project)
	}
	if parsed.BlockerReason != task.BlockerReason {
		t.Errorf("expected blocker reason '%s', got '%s'", task.BlockerReason, parsed.BlockerReason)
	}
	if parsed.Description != task.Description {
		t.Errorf("expected description '%s', got '%s'", task.Description, parsed.Description)
	}
}

func TestParseEmptyTitle(t *testing.T) {
	tmpl := FormatNewTaskTemplate("myproject")
	parsed, err := ParseTaskTemplate(tmpl)
	if err != nil {
		t.Fatalf("ParseTaskTemplate failed: %v", err)
	}

	if parsed.Title != "" {
		t.Errorf("expected empty title, got '%s'", parsed.Title)
	}
}
