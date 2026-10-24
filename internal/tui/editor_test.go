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

func TestParseEditTaskTemplate(t *testing.T) {
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
	}

	allTasks := []*model.Task{task}

	tmpl := FormatEditTaskTemplate(task, allTasks, obs)
	if !strings.Contains(tmpl, "Valgrind detected 48 bytes unfree") {
		t.Errorf("expected observation history in template")
	}
	if !strings.Contains(tmpl, "Virtual Tags:") {
		t.Errorf("expected virtual tags in template")
	}

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
	if len(parsed.Tags) != 2 || parsed.Tags[0] != "mem" || parsed.Tags[1] != "leak" {
		t.Errorf("unexpected tags: %+v", parsed.Tags)
	}
	if len(parsed.Dependencies) != 1 || parsed.Dependencies[0] != 10 {
		t.Errorf("unexpected dependencies: %+v", parsed.Dependencies)
	}
	if parsed.Description != task.Description {
		t.Errorf("expected description '%s', got '%s'", task.Description, parsed.Description)
	}
}
