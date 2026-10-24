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
	if !strings.Contains(tmpl, "Valgrind detected 48 bytes unfree") {
		t.Errorf("expected observation 1 in template")
	}
	if !strings.Contains(tmpl, "Root cause found in pool_release()") {
		t.Errorf("expected observation 2 in template")
	}

	// Modify Note 1 and add New Note
	modifiedInput := strings.Replace(tmpl, "Valgrind detected 48 bytes unfree", "Valgrind detected 64 bytes unfree in slab_alloc", 1)
	modifiedInput = strings.Replace(modifiedInput, `new_note: ""`, "new_note: Verified with ASan run on Linux", 1)

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

func TestParseYAMLMultiLineBlockerReasonAndNotes(t *testing.T) {
	yamlInput := `title: Implement distributed raft consensus
status: blocked
priority: C
project: raft
tags:
  - consensus
  - cluster
depends:
  - 101
  - 102
blocker_reason: |-
  Waiting on network partition simulation results.
  Also blocked by disk sync I/O benchmarks.
new_note: |-
  First line of observation.
  Second line of observation.
notes:
  1: |-
    Multi-line existing note line 1.
    Multi-line existing note line 2.
  2: Single line existing note
description: |-
  This is a multi-line task description.
  It details the full Raft state machine.
`

	parsed, err := ParseTaskTemplate(yamlInput)
	if err != nil {
		t.Fatalf("ParseTaskTemplate failed: %v", err)
	}

	if parsed.Title != "Implement distributed raft consensus" {
		t.Errorf("expected title, got %q", parsed.Title)
	}
	if parsed.Status != model.StatusBlocked {
		t.Errorf("expected status blocked, got %v", parsed.Status)
	}
	if parsed.Priority != model.PriorityCritical {
		t.Errorf("expected priority C, got %v", parsed.Priority)
	}
	expectedBlocker := "Waiting on network partition simulation results.\nAlso blocked by disk sync I/O benchmarks."
	if parsed.BlockerReason != expectedBlocker {
		t.Errorf("expected multi-line blocker reason:\n%q\ngot:\n%q", expectedBlocker, parsed.BlockerReason)
	}
	expectedNewNote := "First line of observation.\nSecond line of observation."
	if parsed.NewNote != expectedNewNote {
		t.Errorf("expected multi-line new note:\n%q\ngot:\n%q", expectedNewNote, parsed.NewNote)
	}
	expectedNote1 := "Multi-line existing note line 1.\nMulti-line existing note line 2."
	if parsed.ExistingNotes[1] != expectedNote1 {
		t.Errorf("expected note 1:\n%q\ngot:\n%q", expectedNote1, parsed.ExistingNotes[1])
	}
	if parsed.ExistingNotes[2] != "Single line existing note" {
		t.Errorf("expected note 2, got %q", parsed.ExistingNotes[2])
	}
	expectedDesc := "This is a multi-line task description.\nIt details the full Raft state machine."
	if parsed.Description != expectedDesc {
		t.Errorf("expected description:\n%q\ngot:\n%q", expectedDesc, parsed.Description)
	}
}

func TestParseMultiLineObservations(t *testing.T) {
	input := `Title: Memory leak diagnosis
Status: in_progress
Priority: H

Note 1: Initial leak detected
  Valgrind reported 64 bytes in slab_alloc
  Fixed by adding free call in pool_release()

Note 2: Second single-line observation

New Note:
First line of multi-line note
Second line of multi-line note
Third line of multi-line note

Description:
This is the description.
`

	parsed, err := ParseTaskTemplate(input)
	if err != nil {
		t.Fatalf("ParseTaskTemplate failed: %v", err)
	}

	expectedNote1 := "Initial leak detected\nValgrind reported 64 bytes in slab_alloc\nFixed by adding free call in pool_release()"
	if parsed.ExistingNotes[1] != expectedNote1 {
		t.Errorf("expected multi-line note 1 to be:\n%q\ngot:\n%q", expectedNote1, parsed.ExistingNotes[1])
	}

	expectedNewNote := "First line of multi-line note\nSecond line of multi-line note\nThird line of multi-line note"
	if parsed.NewNote != expectedNewNote {
		t.Errorf("expected multi-line new note to be:\n%q\ngot:\n%q", expectedNewNote, parsed.NewNote)
	}

	if parsed.Description != "This is the description." {
		t.Errorf("expected description 'This is the description.', got '%s'", parsed.Description)
	}
}
