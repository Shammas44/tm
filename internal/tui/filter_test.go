package tui

import (
	"strings"
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

	// Sort by observation count (Notes) descending
	obsCounts := map[int64]int{
		1: 5,
		2: 0,
		3: 12,
	}
	SortTasks(tasks, SortOrder{Field: SortByNotes, Ascending: false}, obsCounts)
	if tasks[0].ID != 3 || tasks[1].ID != 1 || tasks[2].ID != 2 {
		t.Errorf("expected notes sorting 3 (12), 1 (5), 2 (0); got %d, %d, %d", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}

	// Sort by observation count ascending
	SortTasks(tasks, SortOrder{Field: SortByNotes, Ascending: true}, obsCounts)
	if tasks[0].ID != 2 || tasks[1].ID != 1 || tasks[2].ID != 3 {
		t.Errorf("expected notes sorting 2 (0), 1 (5), 3 (12); got %d, %d, %d", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}

	// Test Column Mapping
	if ColumnToSortField(0) != SortByID || ColumnToSortField(6) != SortByNotes || ColumnToSortField(7) != SortByCreated {
		t.Errorf("unexpected column mapping")
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

func TestWrapText(t *testing.T) {
	multiLine := "Line 1 of description\nLine 2 is somewhat longer and needs wrapping at thirty chars\nLine 3"
	wrapped := wrapText(multiLine, 30, "  ")
	if len(wrapped) < 3 {
		t.Errorf("expected at least 3 lines, got %d: %v", len(wrapped), wrapped)
	}
	for _, l := range wrapped {
		if !strings.HasPrefix(l, "  ") {
			t.Errorf("expected line to start with indent '  ', got '%s'", l)
		}
	}
}

func TestHalfHeightSplit(t *testing.T) {
	m := Model{
		height: 30,
		width:  100,
	}

	halfTop := m.height / 2
	halfBottom := m.height - halfTop

	// Top half: overhead + availableRows should equal halfTop
	overhead := m.topSectionOverhead()
	avail := m.availableRows()
	if overhead+avail != halfTop {
		t.Errorf("expected top section to equal half screen (%d), got overhead (%d) + avail (%d) = %d",
			halfTop, overhead, avail, overhead+avail)
	}

	// Bottom half: separator (1) + previewPaneHeight + footer (1) should equal halfBottom
	paneH := m.previewPaneHeight()
	if 1+paneH+1 != halfBottom {
		t.Errorf("expected bottom section to equal half screen (%d), got 1 + paneH (%d) + 1 = %d",
			halfBottom, paneH, 1+paneH+1)
	}
}

func TestHelpLayoutAndScrolling(t *testing.T) {
	m := Model{
		height: 24,
		width:  100,
	}

	lines := m.helpContentLines()
	if len(lines) == 0 {
		t.Fatalf("expected help content lines, got 0")
	}

	maxOffset := m.maxHelpScrollOffset()
	visHeight := m.helpVisibleHeight()
	if maxOffset != len(lines)-visHeight {
		t.Errorf("expected maxOffset %d, got %d", len(lines)-visHeight, maxOffset)
	}

	// Ensure help render does not contain borders
	helpView := m.renderFullScreenHelp()
	if strings.Contains(helpView, "╭") || strings.Contains(helpView, "╮") || strings.Contains(helpView, "╰") || strings.Contains(helpView, "╯") {
		t.Errorf("expected help view to have no rounded popup border")
	}
}

func TestRenderDetailPaneFormat(t *testing.T) {
	taskID := int64(42)
	task := &model.Task{
		ID:            taskID,
		Title:         "Refactor core",
		BlockerReason: "Waiting on PR #12\nReview pending",
		Description:   "First line\nSecond line",
	}

	obs := []*model.Observation{
		{ID: 1, TaskID: &taskID, Content: "Obs line 1\nObs line 2", CreatedAt: time.Now()},
		{ID: 2, TaskID: &taskID, Content: "Single line obs", CreatedAt: time.Now()},
	}

	m := Model{
		height:          30,
		width:           100,
		tasks:           []*model.Task{task},
		allTasks:        []*model.Task{task},
		selectedTaskObs: obs,
		cursor:          0,
	}

	rendered := m.renderDetailPane()

	// Verify Blocker Reason header and indented lines
	if !strings.Contains(rendered, "Blocker Reason:\n    Waiting on PR #12\n    Review pending") {
		t.Errorf("expected Blocker Reason header and 4-space indented values, got:\n%s", rendered)
	}

	// Verify Description header and indented lines
	if !strings.Contains(rendered, "Description:\n    First line\n    Second line") {
		t.Errorf("expected Description header and 4-space indented values, got:\n%s", rendered)
	}

	// Verify Observation header
	if !strings.Contains(rendered, "Observation (2 recorded):") {
		t.Errorf("expected 'Observation (2 recorded):', got:\n%s", rendered)
	}

	// Verify observation bullet and indented lines
	if !strings.Contains(rendered, "- [") || !strings.Contains(rendered, "(#1)\n  Obs line 1\n  Obs line 2") {
		t.Errorf("expected observation formatted with bullet and 2-space indented lines, got:\n%s", rendered)
	}
}
