package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"taskmanager/internal/db"
	"taskmanager/internal/model"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	s := New(database)
	cleanup := func() {
		database.Close()
	}
	return s, cleanup
}

func TestTaskCRUDAndStateTransitions(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Create Tasks - verify autoincrement IDs
	t1, err := s.CreateTask(ctx, &model.Task{
		Title:       "Task One",
		Description: "First task description",
		Priority:    model.PriorityHigh,
		Project:     "testproj",
	})
	if err != nil {
		t.Fatalf("CreateTask 1 failed: %v", err)
	}
	if t1.ID != 1 {
		t.Errorf("expected ID 1, got %d", t1.ID)
	}

	t2, err := s.CreateTask(ctx, &model.Task{
		Title:    "Task Two",
		Priority: model.PriorityMedium,
		Project:  "testproj",
	})
	if err != nil {
		t.Fatalf("CreateTask 2 failed: %v", err)
	}
	if t2.ID != 2 {
		t.Errorf("expected ID 2, got %d", t2.ID)
	}

	// 2. Set active task
	err = s.SetActiveTask(ctx, 1)
	if err != nil {
		t.Fatalf("SetActiveTask failed: %v", err)
	}

	active, err := s.GetActiveTask(ctx, "testproj")
	if err != nil {
		t.Fatalf("GetActiveTask failed: %v", err)
	}
	if active.ID != 1 || !active.Active || active.Status != model.StatusStarted {
		t.Errorf("expected active task #1 started, got: %+v", active)
	}

	// 3. Block task
	err = s.UpdateTaskStatus(ctx, 1, model.StatusBlocked, "Waiting on PR")
	if err != nil {
		t.Fatalf("UpdateTaskStatus blocked failed: %v", err)
	}
	blocked, err := s.GetTask(ctx, 1)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if blocked.Status != model.StatusBlocked || blocked.Active || blocked.Blocker != "Waiting on PR" {
		t.Errorf("unexpected blocked task: %+v", blocked)
	}

	// 4. Complete task
	err = s.UpdateTaskStatus(ctx, 1, model.StatusCompleted, "")
	if err != nil {
		t.Fatalf("UpdateTaskStatus completed failed: %v", err)
	}
	completed, err := s.GetTask(ctx, 1)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if completed.Status != model.StatusCompleted || completed.CompletedAt == nil {
		t.Errorf("expected completed task with timestamp, got: %+v", completed)
	}

	// 5. Check Backlog counts
	counts, err := s.GetBacklogCounts(ctx, "testproj")
	if err != nil {
		t.Fatalf("GetBacklogCounts failed: %v", err)
	}
	if counts.Pending != 1 || counts.Completed != 1 || counts.Started != 0 || counts.Blocked != 0 {
		t.Errorf("unexpected backlog counts: %+v", counts)
	}
}

func TestDeleteTaskPermanently(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	task, err := s.CreateTask(ctx, &model.Task{
		Title:   "Task to purge",
		Project: "purgeproj",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	_, err = s.AddObservation(ctx, nil, &task.ID, "Obs for purge task")
	if err != nil {
		t.Fatalf("AddObservation failed: %v", err)
	}

	if err := s.DeleteTaskPermanently(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTaskPermanently failed: %v", err)
	}

	// Should not be found at all
	_, err = s.GetTask(ctx, task.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSessionLifecycleAndCheckpoints(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Create a task first
	t1, err := s.CreateTask(ctx, &model.Task{
		Title:   "Session test task",
		Project: "projA",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// 2. Start Session
	session, err := s.StartSession(ctx, "agent-claude", &t1.ID)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	active, err := s.GetActiveSession(ctx)
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if active.ID != session.ID || active.ActiveTaskID == nil || *active.ActiveTaskID != t1.ID {
		t.Errorf("unexpected active session: %+v", active)
	}

	// 3. Update Checkpoint
	err = s.UpdateCheckpoint(ctx, session.ID, &t1.ID, "Refactoring math functions")
	if err != nil {
		t.Fatalf("UpdateCheckpoint failed: %v", err)
	}

	activeAfter, err := s.GetActiveSession(ctx)
	if err != nil {
		t.Fatalf("GetActiveSession after checkpoint failed: %v", err)
	}
	if activeAfter.Summary != "Refactoring math functions" {
		t.Errorf("expected summary updated to checkpoint note, got: %s", activeAfter.Summary)
	}

	// 4. Close Session
	err = s.CloseSession(ctx, session.ID, "Finished math refactor", "Add benchmarks")
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	last, err := s.GetLastSession(ctx)
	if err != nil {
		t.Fatalf("GetLastSession failed: %v", err)
	}
	if last.Status != model.SessionStatusCompleted || last.NextAction != "Add benchmarks" {
		t.Errorf("unexpected last session: %+v", last)
	}
}

func TestObservationsAndFTS5Search(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create a task first
	task, err := s.CreateTask(ctx, &model.Task{
		Title: "Task for observations",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Add observations
	obs1, err := s.AddObservation(ctx, nil, &task.ID, "ASan detects heap-use-after-free in parser line 42")
	if err != nil {
		t.Fatalf("AddObservation 1 failed: %v", err)
	}

	obs2, err := s.AddObservation(ctx, nil, nil, "CMake requires Ninja 1.11+ for sanitizers")
	if err != nil {
		t.Fatalf("AddObservation 2 failed: %v", err)
	}

	// Search 'ASan'
	results, err := s.SearchObservations(ctx, "ASan")
	if err != nil {
		t.Fatalf("SearchObservations 'ASan' failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != obs1.ID {
		t.Fatalf("expected 1 result matching 'ASan', got %d", len(results))
	}

	// Search 'Ninja'
	results, err = s.SearchObservations(ctx, "Ninja")
	if err != nil {
		t.Fatalf("SearchObservations 'Ninja' failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != obs2.ID {
		t.Fatalf("expected 1 result matching 'Ninja', got %d", len(results))
	}
}

func TestConcurrentAccess(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := s.CreateTask(ctx, &model.Task{
					Title:   fmt.Sprintf("Worker %d Task %d", workerID, j),
					Project: "concurrent-test",
				})
				if err != nil {
					t.Errorf("concurrent CreateTask failed: %v", err)
					return
				}

				_, err = s.ListTasks(ctx, TaskFilter{Project: "concurrent-test"})
				if err != nil {
					t.Errorf("concurrent ListTasks failed: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	tasks, err := s.ListTasks(ctx, TaskFilter{Project: "concurrent-test"})
	if err != nil {
		t.Fatalf("final ListTasks failed: %v", err)
	}
	expected := workers * iterations
	if len(tasks) != expected {
		t.Errorf("expected %d tasks created concurrently, got %d", expected, len(tasks))
	}
}
