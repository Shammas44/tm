package tui

import (
	"testing"
	"time"

	"taskmanager/internal/model"
)

func TestSortTasks(t *testing.T) {
	t1 := &model.Task{ID: 1, Priority: model.PriorityLow, CreatedAt: time.Now().Add(-1 * time.Hour)}
	t2 := &model.Task{ID: 2, Priority: model.PriorityCritical, CreatedAt: time.Now()}
	t3 := &model.Task{ID: 3, Priority: model.PriorityHigh, CreatedAt: time.Now().Add(-2 * time.Hour)}

	tasks := []*model.Task{t1, t2, t3}

	// Sort by priority descending
	SortTasks(tasks, SortOrder{Field: SortByPriority, Ascending: false})
	if tasks[0].ID != 2 || tasks[1].ID != 3 || tasks[2].ID != 1 {
		t.Errorf("expected priority sorting 2, 3, 1; got %d, %d, %d", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}

	// Sort by ID ascending
	SortTasks(tasks, SortOrder{Field: SortByID, Ascending: true})
	if tasks[0].ID != 1 || tasks[1].ID != 2 || tasks[2].ID != 3 {
		t.Errorf("expected ID sorting 1, 2, 3; got %d, %d, %d", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestFilterWithVirtualTags(t *testing.T) {
	depTask := &model.Task{ID: 10, Status: model.StatusPending}
	t1 := &model.Task{
		ID:           1,
		Title:        "Blocked task",
		Status:       model.StatusPending,
		Dependencies: []int64{10},
	}
	t2 := &model.Task{
		ID:     2,
		Title:  "Ready task",
		Status: model.StatusPending,
		Tags:   []string{"frontend"},
	}

	allTasks := []*model.Task{depTask, t1, t2}

	// Filter by virtual tag +READY
	critReady := ParseSearchQuery("+READY")
	if MatchesFilter(t1, allTasks, false, critReady) {
		t.Errorf("t1 depends on incomplete task 10, should not match +READY")
	}
	if !MatchesFilter(t2, allTasks, false, critReady) {
		t.Errorf("t2 has no blockers or dependencies, should match +READY")
	}

	// Filter by virtual tag +BLOCKED
	critBlocked := ParseSearchQuery("+BLOCKED")
	if !MatchesFilter(t1, allTasks, false, critBlocked) {
		t.Errorf("t1 has incomplete dependency 10, should match +BLOCKED")
	}

	// Filter by user tag +frontend
	critTag := ParseSearchQuery("+frontend")
	if !MatchesFilter(t2, allTasks, false, critTag) {
		t.Errorf("t2 has tag 'frontend', should match +frontend")
	}
}
