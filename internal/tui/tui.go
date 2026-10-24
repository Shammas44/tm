package tui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	headerCellStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	headerSelectedColStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("240"))

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("237"))

	selectedCellStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("14")) // Bright Cyan block for selected cell

	inProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	completedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	blockedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	pendingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	cancelledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	criticalPrioStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	highPrioStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	mediumPrioStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	lowPrioStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	paneBarHead  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	paneSubStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cmdPrompt    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))

	// Column Widths with guaranteed 2-character gap separation
	colGap          = "  "
	colMarkWidth    = 2
	colIDWidth      = 4
	colStatusWidth  = 11
	colPriWidth     = 3
	colProjectWidth = 10
	colTagsWidth    = 12
	colDepsWidth    = 6
	colNotesWidth   = 5  // Observation count column
	colCreatedWidth = 16 // Human-readable timestamp e.g. "2006-01-02 15:04"
)

type ViewMode int

const (
	ModeList ViewMode = iota
	ModeCommand
	ModeSearch
	ModeBlockInput
	ModeHelp
	ModeObservations
)

type editorDoneMsg struct {
	mode     string // "create" or "edit"
	taskID   int64
	tempPath string
	err      error
}

type Model struct {
	store            *store.Store
	tasks            []*model.Task
	allTasks         []*model.Task
	obsCounts        map[int64]int
	activeTab        int // 0: All, 1: Ready, 2: Pending, 3: In Progress, 4: Blocked, 5: Completed
	cursor           int // Selected row index
	selectedCol      int // Selected column index (0 to TotalColumns-1)
	scrollOffset     int // Top visible row index
	helpScrollOffset int
	previewScroll    int
	sortOrder        SortOrder
	viewMode         ViewMode
	observations     []*model.Observation // For dedicated modal
	selectedTaskObs  []*model.Observation // For bottom readonly inspection pane
	searchQuery      string
	commandInput     string // For Vim command mode ':'
	blockInput       string
	statusMsg        string
	lastKey          rune
	lastKeyPressTime time.Time
	width            int
	height           int
}

var tabs = []string{"All", "Ready", "Pending", "In Progress", "Blocked", "Completed"}

func New(st *store.Store) Model {
	m := Model{
		store:       st,
		activeTab:   0,
		viewMode:    ModeList,
		selectedCol: 0,
		sortOrder:   SortOrder{Field: SortByID, Ascending: true},
		width:       80,
		height:      24,
	}
	m.reloadTasks()
	return m
}

func (m *Model) reloadTasks() {
	ctx := context.Background()
	all, err := m.store.ListTasks(ctx, store.TaskFilter{IncludeDeleted: false})
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error loading tasks: %v", err)
		return
	}
	m.allTasks = all

	counts, _ := m.store.GetObservationCounts(ctx)
	m.obsCounts = counts

	m.filterTasks()
	m.loadSelectedTaskObs()
}

func (m *Model) loadSelectedTaskObs() {
	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		m.selectedTaskObs = nil
		return
	}
	ctx := context.Background()
	taskID := m.tasks[m.cursor].ID
	obs, err := m.store.ListObservations(ctx, store.ObservationFilter{TaskID: &taskID})
	if err == nil {
		m.selectedTaskObs = obs
	} else {
		m.selectedTaskObs = nil
	}
}

func (m *Model) filterTasks() {
	criteria := ParseSearchQuery(m.searchQuery)
	var filtered []*model.Task

	for _, t := range m.allTasks {
		vtags := model.ComputeVirtualTags(t, m.allTasks, false)
		vtagMap := make(map[string]bool)
		for _, vt := range vtags {
			vtagMap[vt] = true
		}

		switch m.activeTab {
		case 1: // Ready
			if !vtagMap["READY"] {
				continue
			}
		case 2: // Pending
			if t.Status != model.StatusPending {
				continue
			}
		case 3: // In Progress
			if t.Status != model.StatusInProgress {
				continue
			}
		case 4: // Blocked
			if !vtagMap["BLOCKED"] {
				continue
			}
		case 5: // Completed
			if t.Status != model.StatusCompleted {
				continue
			}
		}

		if MatchesFilter(t, m.allTasks, false, criteria) {
			filtered = append(filtered, t)
		}
	}

	SortTasks(filtered, m.sortOrder, m.obsCounts)
	m.tasks = filtered

	if m.cursor >= len(m.tasks) {
		m.cursor = max(0, len(m.tasks)-1)
	}
	m.adjustScroll()
}

func (m *Model) adjustScroll() {
	visibleRows := m.availableRows()
	if visibleRows <= 0 {
		return
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+visibleRows {
		m.scrollOffset = m.cursor - visibleRows + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m Model) previewPaneHeight() int {
	if m.height <= 18 {
		return 5
	}
	p := (m.height - 8) / 3
	if p < 5 {
		return 5
	}
	if p > 10 {
		return 10
	}
	return p
}

func (m Model) availableRows() int {
	// Total height minus:
	// - Header lines: 2
	// - Search / filter line (if any): 2
	// - Table headers + separator: 2
	// - Preview pane separator + content: 1 + previewHeight
	// - Status / Command line: 1
	overhead := 2 + 2 + 1 + m.previewPaneHeight() + 1
	if m.searchQuery != "" || m.viewMode == ModeSearch {
		overhead += 2
	}
	rows := m.height - overhead
	if rows < 3 {
		return 3
	}
	return rows
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	ctx := context.Background()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustScroll()
		return m, nil

	case editorDoneMsg:
		contentBytes, readErr := os.ReadFile(msg.tempPath)
		_ = os.Remove(msg.tempPath)

		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Editor exited with error: %v", msg.err)
			return m, nil
		}
		if readErr != nil {
			m.statusMsg = fmt.Sprintf("Failed to read editor output: %v", readErr)
			return m, nil
		}

		parsed, err := ParseTaskTemplate(string(contentBytes))
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to parse task: %v", err)
			return m, nil
		}

		if strings.TrimSpace(parsed.Title) == "" {
			m.statusMsg = "Task creation/edit cancelled (empty title)"
			return m, nil
		}

		activeSession, _ := m.store.GetActiveSession(ctx)
		var sessionID *string
		if activeSession != nil {
			sessionID = &activeSession.ID
		}

		if msg.mode == "create" {
			newTask := &model.Task{
				Title:         parsed.Title,
				Status:        parsed.Status,
				Priority:      parsed.Priority,
				Project:       parsed.Project,
				Description:   parsed.Description,
				BlockerReason: parsed.BlockerReason,
				Tags:          parsed.Tags,
				Dependencies:  parsed.Dependencies,
			}
			created, err := m.store.CreateTask(ctx, newTask)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Failed to create task: %v", err)
				return m, nil
			}
			if created.Status == model.StatusInProgress {
				_ = m.store.SetActiveTask(ctx, created.ID)
			}

			if strings.TrimSpace(parsed.NewNote) != "" {
				_, _ = m.store.AddObservation(ctx, sessionID, &created.ID, parsed.NewNote)
			}

			if activeSession != nil {
				_ = m.store.UpdateCheckpoint(ctx, activeSession.ID, &created.ID, fmt.Sprintf("Created task #%d: %s", created.ID, created.Title))
			}

			m.statusMsg = fmt.Sprintf("Created task #%d: %s", created.ID, created.Title)
			m.reloadTasks()
			return m, nil
		}

		if msg.mode == "edit" {
			existing, err := m.store.GetTask(ctx, msg.taskID)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Failed to find task #%d: %v", msg.taskID, err)
				return m, nil
			}
			existing.Title = parsed.Title
			existing.Status = parsed.Status
			existing.Priority = parsed.Priority
			existing.Project = parsed.Project
			existing.Description = parsed.Description
			existing.BlockerReason = parsed.BlockerReason
			existing.Tags = parsed.Tags
			existing.Dependencies = parsed.Dependencies

			if existing.Status == model.StatusInProgress {
				_ = m.store.SetActiveTask(ctx, existing.ID)
				_ = m.store.UpdateTask(ctx, existing)
			} else {
				if err := m.store.UpdateTask(ctx, existing); err != nil {
					m.statusMsg = fmt.Sprintf("Failed to update task #%d: %v", existing.ID, err)
					return m, nil
				}
			}

			// Modify / delete existing observations based on editor changes
			origObs, _ := m.store.ListObservations(ctx, store.ObservationFilter{TaskID: &existing.ID})
			for _, o := range origObs {
				if newContent, found := parsed.ExistingNotes[o.ID]; found {
					trimmedContent := strings.TrimSpace(newContent)
					if trimmedContent == "" {
						_ = m.store.DeleteObservation(ctx, o.ID)
					} else if trimmedContent != o.Content {
						_ = m.store.UpdateObservation(ctx, o.ID, trimmedContent)
					}
				} else {
					_ = m.store.DeleteObservation(ctx, o.ID)
				}
			}

			if strings.TrimSpace(parsed.NewNote) != "" {
				_, _ = m.store.AddObservation(ctx, sessionID, &existing.ID, parsed.NewNote)
			}

			if activeSession != nil {
				_ = m.store.UpdateCheckpoint(ctx, activeSession.ID, &existing.ID, fmt.Sprintf("Updated task #%d: %s", existing.ID, existing.Title))
			}

			m.statusMsg = fmt.Sprintf("Updated task #%d: %s", existing.ID, existing.Title)
			m.reloadTasks()
			return m, nil
		}

	case tea.KeyMsg:
		if m.viewMode == ModeHelp {
			switch msg.String() {
			case "esc", "q", "?", "enter":
				m.viewMode = ModeList
				return m, nil
			case "j", "down":
				m.helpScrollOffset++
				return m, nil
			case "k", "up":
				if m.helpScrollOffset > 0 {
					m.helpScrollOffset--
				}
				return m, nil
			case "g", "home":
				m.helpScrollOffset = 0
				return m, nil
			case "G", "end":
				m.helpScrollOffset = 30
				return m, nil
			case "pgdown":
				m.helpScrollOffset += 10
				return m, nil
			case "pgup":
				if m.helpScrollOffset > 10 {
					m.helpScrollOffset -= 10
				} else {
					m.helpScrollOffset = 0
				}
				return m, nil
			}
			return m, nil
		}

		if m.viewMode == ModeObservations {
			switch msg.String() {
			case "esc", "q", "v", "enter":
				m.viewMode = ModeList
				return m, nil
			}
			return m, nil
		}

		if m.viewMode == ModeSearch {
			switch msg.String() {
			case "esc":
				m.searchQuery = ""
				m.viewMode = ModeList
				m.filterTasks()
				m.loadSelectedTaskObs()
				return m, nil
			case "enter":
				m.viewMode = ModeList
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.filterTasks()
					m.loadSelectedTaskObs()
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					m.filterTasks()
					m.loadSelectedTaskObs()
				}
				return m, nil
			}
		}

		if m.viewMode == ModeCommand {
			switch msg.String() {
			case "esc":
				m.commandInput = ""
				m.viewMode = ModeList
				return m, nil
			case "enter":
				cmdStr := strings.TrimSpace(m.commandInput)
				m.commandInput = ""
				m.viewMode = ModeList
				return m.executeCommand(cmdStr)
			case "backspace":
				if len(m.commandInput) > 0 {
					m.commandInput = m.commandInput[:len(m.commandInput)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.commandInput += msg.String()
				}
				return m, nil
			}
		}

		if m.viewMode == ModeBlockInput {
			switch msg.String() {
			case "esc":
				m.blockInput = ""
				m.viewMode = ModeList
				return m, nil
			case "enter":
				if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
					t := m.tasks[m.cursor]
					reason := strings.TrimSpace(m.blockInput)
					if reason == "" {
						reason = "Blocked"
					}
					_ = m.store.UpdateTaskStatus(ctx, t.ID, model.StatusBlocked, reason)
					m.statusMsg = fmt.Sprintf("Blocked task #%d: %s", t.ID, reason)
					m.blockInput = ""
					m.viewMode = ModeList
					m.reloadTasks()
				}
				return m, nil
			case "backspace":
				if len(m.blockInput) > 0 {
					m.blockInput = m.blockInput[:len(m.blockInput)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.blockInput += msg.String()
				}
				return m, nil
			}
		}

		// ModeList keys
		// Requirement 3: Vim 'gg' and 'G' navigation
		if msg.String() == "g" {
			now := time.Now()
			if m.lastKey == 'g' && now.Sub(m.lastKeyPressTime) < 800*time.Millisecond {
				m.cursor = 0
				m.scrollOffset = 0
				m.lastKey = 0
				m.adjustScroll()
				m.loadSelectedTaskObs()
				return m, nil
			}
			m.lastKey = 'g'
			m.lastKeyPressTime = now
			return m, nil
		}

		// Any other key resets the pending 'g' buffer
		m.lastKey = 0

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "?": // Help modal
			m.viewMode = ModeHelp
			m.helpScrollOffset = 0
			return m, nil

		case ":": // Requirement 8: Command mode
			m.viewMode = ModeCommand
			m.commandInput = ""
			return m, nil

		// Requirement 1: Vertical line navigation
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		// Requirement 1: Horizontal column navigation
		case "left", "h":
			if m.selectedCol > 0 {
				m.selectedCol--
			}

		case "right", "l":
			if m.selectedCol < TotalColumns-1 {
				m.selectedCol++
			}

		case "home":
			m.cursor = 0
			m.scrollOffset = 0
			m.loadSelectedTaskObs()

		case "G", "end":
			if len(m.tasks) > 0 {
				m.cursor = len(m.tasks) - 1
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		case "pgup":
			m.cursor = max(0, m.cursor-m.availableRows())
			m.adjustScroll()
			m.loadSelectedTaskObs()

		case "pgdown":
			if len(m.tasks) > 0 {
				m.cursor = min(len(m.tasks)-1, m.cursor+m.availableRows())
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		case "tab":
			m.activeTab = (m.activeTab + 1) % len(tabs)
			m.cursor = 0
			m.scrollOffset = 0
			m.filterTasks()
			m.loadSelectedTaskObs()

		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(tabs)) % len(tabs)
			m.cursor = 0
			m.scrollOffset = 0
			m.filterTasks()
			m.loadSelectedTaskObs()

		case "1", "2", "3", "4", "5", "6":
			tabIdx := int(msg.String()[0] - '1')
			if tabIdx < len(tabs) {
				m.activeTab = tabIdx
				m.cursor = 0
				m.scrollOffset = 0
				m.filterTasks()
				m.loadSelectedTaskObs()
			}

		// Requirement 1: Sort by selected column, toggle direction if pressed again
		case "s":
			targetField := ColumnToSortField(m.selectedCol)
			if m.sortOrder.Field == targetField {
				m.sortOrder.Ascending = !m.sortOrder.Ascending
			} else {
				m.sortOrder.Field = targetField
				m.sortOrder.Ascending = DefaultAscendingForField(targetField)
			}
			SortTasks(m.tasks, m.sortOrder, m.obsCounts)
			m.statusMsg = fmt.Sprintf("Sorted by %s", m.sortOrder.Label())

		case "a", "c": // Add / Create task using $EDITOR
			return m.openCreateEditor()

		case "e": // Edit selected task using $EDITOR
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				return m.openEditEditor(m.tasks[m.cursor].ID)
			}

		case "enter": // Start task
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				t := m.tasks[m.cursor]
				if err := m.store.SetActiveTask(ctx, t.ID); err == nil {
					m.statusMsg = fmt.Sprintf("Started task #%d (%s)", t.ID, t.Title)
					m.reloadTasks()
				}
			}

		case "x": // Mark done
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				t := m.tasks[m.cursor]
				if err := m.store.UpdateTaskStatus(ctx, t.ID, model.StatusCompleted, ""); err == nil {
					m.statusMsg = fmt.Sprintf("Completed task #%d (%s)", t.ID, t.Title)
					m.reloadTasks()
				}
			}

		case "b": // Block task
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				m.blockInput = ""
				m.viewMode = ModeBlockInput
			}

		case "v": // View observations modal
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				t := m.tasks[m.cursor]
				obs, _ := m.store.ListObservations(ctx, store.ObservationFilter{TaskID: &t.ID})
				m.observations = obs
				m.viewMode = ModeObservations
			}

		case "/": // Search
			m.viewMode = ModeSearch
			m.searchQuery = ""
		}
	}

	return m, nil
}

func (m Model) openCreateEditor() (Model, tea.Cmd) {
	tmpFile, err := os.CreateTemp("", "tm-create-*.txt")
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to create temporary editor file: %v", err)
		return m, nil
	}
	defaultProject := ""
	if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
		defaultProject = m.tasks[m.cursor].Project
	}
	tmpl := FormatNewTaskTemplate(defaultProject)
	_, _ = tmpFile.WriteString(tmpl)
	_ = tmpFile.Close()

	cmd := ResolveEditorCommand(tmpFile.Name())
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{
			mode:     "create",
			tempPath: tmpFile.Name(),
			err:      err,
		}
	})
}

func (m Model) openEditEditor(taskID int64) (Model, tea.Cmd) {
	ctx := context.Background()
	task, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Task #%d not found: %v", taskID, err)
		return m, nil
	}
	obs, _ := m.store.ListObservations(ctx, store.ObservationFilter{TaskID: &task.ID})
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("tm-edit-%d-*.txt", task.ID))
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to create temporary editor file: %v", err)
		return m, nil
	}
	tmpl := FormatEditTaskTemplate(task, m.allTasks, obs)
	_, _ = tmpFile.WriteString(tmpl)
	_ = tmpFile.Close()

	cmd := ResolveEditorCommand(tmpFile.Name())
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{
			mode:     "edit",
			taskID:   task.ID,
			tempPath: tmpFile.Name(),
			err:      err,
		}
	})
}

// Requirement 8: Command mode parser and dispatcher
func (m Model) executeCommand(cmdStr string) (Model, tea.Cmd) {
	if cmdStr == "" {
		return m, nil
	}

	parts := strings.Fields(cmdStr)
	action := strings.ToLower(parts[0])
	args := parts[1:]
	ctx := context.Background()

	var selectedID int64
	if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
		selectedID = m.tasks[m.cursor].ID
	}

	switch action {
	case "q", "quit", "exit":
		return m, tea.Quit

	case "w":
		m.statusMsg = "Saved checkpoint."
		return m, nil

	case "wq", "x":
		return m, tea.Quit

	case "h", "help", "?":
		m.viewMode = ModeHelp
		m.helpScrollOffset = 0
		return m, nil

	case "add", "new":
		if len(args) == 0 {
			return m.openCreateEditor()
		}
		title := strings.Join(args, " ")
		newTask := &model.Task{
			Title:    title,
			Status:   model.StatusPending,
			Priority: model.PriorityMedium,
		}
		created, err := m.store.CreateTask(ctx, newTask)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to add task: %v", err)
		} else {
			m.statusMsg = fmt.Sprintf("Created task #%d: %s", created.ID, created.Title)
			m.reloadTasks()
		}
		return m, nil

	case "start":
		id := selectedID
		if len(args) > 0 {
			parsedID, err := strconv.ParseInt(args[0], 10, 64)
			if err == nil {
				id = parsedID
			}
		}
		if id == 0 {
			m.statusMsg = "No task selected."
			return m, nil
		}
		if err := m.store.SetActiveTask(ctx, id); err != nil {
			m.statusMsg = fmt.Sprintf("Error starting task #%d: %v", id, err)
		} else {
			m.statusMsg = fmt.Sprintf("Started task #%d", id)
			m.reloadTasks()
		}
		return m, nil

	case "done":
		id := selectedID
		if len(args) > 0 {
			parsedID, err := strconv.ParseInt(args[0], 10, 64)
			if err == nil {
				id = parsedID
			}
		}
		if id == 0 {
			m.statusMsg = "No task selected."
			return m, nil
		}
		if err := m.store.UpdateTaskStatus(ctx, id, model.StatusCompleted, ""); err != nil {
			m.statusMsg = fmt.Sprintf("Error completing task #%d: %v", id, err)
		} else {
			m.statusMsg = fmt.Sprintf("Completed task #%d", id)
			m.reloadTasks()
		}
		return m, nil

	case "block":
		id := selectedID
		reason := "Blocked"
		if len(args) > 0 {
			if parsedID, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				id = parsedID
				if len(args) > 1 {
					reason = strings.Join(args[1:], " ")
				}
			} else {
				reason = strings.Join(args, " ")
			}
		}
		if id == 0 {
			m.statusMsg = "No task selected."
			return m, nil
		}
		if err := m.store.UpdateTaskStatus(ctx, id, model.StatusBlocked, reason); err != nil {
			m.statusMsg = fmt.Sprintf("Error blocking task #%d: %v", id, err)
		} else {
			m.statusMsg = fmt.Sprintf("Blocked task #%d: %s", id, reason)
			m.reloadTasks()
		}
		return m, nil

	case "log", "note":
		id := selectedID
		var note string
		if len(args) > 0 {
			if parsedID, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				id = parsedID
				note = strings.Join(args[1:], " ")
			} else {
				note = strings.Join(args, " ")
			}
		}
		if id == 0 || note == "" {
			m.statusMsg = "Usage: :log [id] <note content>"
			return m, nil
		}
		activeSession, _ := m.store.GetActiveSession(ctx)
		var sessionID *string
		if activeSession != nil {
			sessionID = &activeSession.ID
		}
		_, err := m.store.AddObservation(ctx, sessionID, &id, note)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to log note: %v", err)
		} else {
			m.statusMsg = fmt.Sprintf("Logged note on task #%d", id)
			m.reloadTasks()
		}
		return m, nil

	case "tag":
		id := selectedID
		var tagArgs []string
		if len(args) > 0 {
			if parsedID, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				id = parsedID
				tagArgs = args[1:]
			} else {
				tagArgs = args
			}
		}
		if id == 0 || len(tagArgs) == 0 {
			m.statusMsg = "Usage: :tag [id] +tag -tag"
			return m, nil
		}
		task, err := m.store.GetTask(ctx, id)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Task #%d not found: %v", id, err)
			return m, nil
		}
		tagMap := make(map[string]bool)
		for _, t := range task.Tags {
			tagMap[t] = true
		}
		for _, t := range tagArgs {
			if strings.HasPrefix(t, "-") {
				delete(tagMap, strings.TrimPrefix(t, "-"))
			} else {
				tagMap[strings.TrimPrefix(t, "+")] = true
			}
		}
		var newTags []string
		for t := range tagMap {
			newTags = append(newTags, t)
		}
		task.Tags = newTags
		_ = m.store.UpdateTask(ctx, task)
		m.statusMsg = fmt.Sprintf("Updated task #%d tags: %s", id, strings.Join(newTags, ", "))
		m.reloadTasks()
		return m, nil

	case "dep":
		id := selectedID
		var depArgs []string
		if len(args) > 0 {
			if parsedID, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				id = parsedID
				depArgs = args[1:]
			} else {
				depArgs = args
			}
		}
		if id == 0 || len(depArgs) == 0 {
			m.statusMsg = "Usage: :dep [id] +depID -depID"
			return m, nil
		}
		task, err := m.store.GetTask(ctx, id)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Task #%d not found: %v", id, err)
			return m, nil
		}
		depMap := make(map[int64]bool)
		for _, d := range task.Dependencies {
			depMap[d] = true
		}
		for _, arg := range depArgs {
			if strings.HasPrefix(arg, "-") {
				if d, err := strconv.ParseInt(strings.TrimPrefix(arg, "-"), 10, 64); err == nil {
					delete(depMap, d)
				}
			} else {
				if d, err := strconv.ParseInt(strings.TrimPrefix(arg, "+"), 10, 64); err == nil {
					if d != id {
						depMap[d] = true
					}
				}
			}
		}
		var newDeps []int64
		for d := range depMap {
			newDeps = append(newDeps, d)
		}
		task.Dependencies = newDeps
		_ = m.store.UpdateTask(ctx, task)
		m.statusMsg = fmt.Sprintf("Updated dependencies on task #%d", id)
		m.reloadTasks()
		return m, nil

	case "edit":
		id := selectedID
		if len(args) > 0 {
			if parsedID, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				id = parsedID
			}
		}
		if id == 0 {
			m.statusMsg = "No task selected."
			return m, nil
		}
		return m.openEditEditor(id)

	case "del", "delete", "rm":
		id := selectedID
		if len(args) > 0 {
			if parsedID, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				id = parsedID
			}
		}
		if id == 0 {
			m.statusMsg = "No task selected."
			return m, nil
		}
		if err := m.store.DeleteTask(ctx, id); err != nil {
			m.statusMsg = fmt.Sprintf("Failed to delete task #%d: %v", id, err)
		} else {
			m.statusMsg = fmt.Sprintf("Deleted task #%d", id)
			m.reloadTasks()
		}
		return m, nil

	default:
		m.statusMsg = fmt.Sprintf("Unknown command: :%s (press ? for help)", action)
		return m, nil
	}
}

func (m Model) View() string {
	if m.viewMode == ModeHelp {
		return m.renderFullScreenHelp()
	}

	var s strings.Builder

	// 1. Header: Active category banner & continuous browsing range info
	s.WriteString(titleStyle.Render("TaskManager"))
	s.WriteString(" ")

	currentTabName := tabs[m.activeTab]
	currentTabCount := m.countForTab(m.activeTab)
	selectedTabBanner := fmt.Sprintf("Category: %s (%d tasks)", currentTabName, currentTabCount)
	s.WriteString(activeTabStyle.Render(selectedTabBanner))
	s.WriteString(" ")

	// Requirement 2: Continuous browsing range info
	rangeInfo := ""
	if len(m.tasks) > 0 {
		visibleEnd := min(len(m.tasks), m.scrollOffset+m.availableRows())
		rangeInfo = fmt.Sprintf("[%d-%d of %d] • Sort: %s", m.scrollOffset+1, visibleEnd, len(m.tasks), m.sortOrder.Label())
	} else {
		rangeInfo = fmt.Sprintf("[0 tasks] • Sort: %s", m.sortOrder.Label())
	}
	s.WriteString(helpStyle.Render(rangeInfo))
	s.WriteString("\n\n")

	// 2. Search bar if open
	if m.viewMode == ModeSearch {
		s.WriteString(fmt.Sprintf("Search: %s█\n\n", m.searchQuery))
	} else if m.searchQuery != "" {
		s.WriteString(fmt.Sprintf("Filter: '%s' (press / to edit, esc to clear)\n\n", m.searchQuery))
	}

	// 3. Other modals
	if m.viewMode == ModeBlockInput {
		t := m.tasks[m.cursor]
		s.WriteString(fmt.Sprintf("Enter block reason for #%d '%s': %s█\n(press Enter to confirm, Esc to cancel)\n\n",
			t.ID, t.Title, m.blockInput))
		return s.String()
	}

	if m.viewMode == ModeObservations {
		t := m.tasks[m.cursor]
		s.WriteString(fmt.Sprintf("=== Observations for Task #%d: %s ===\n\n", t.ID, t.Title))
		if len(m.observations) == 0 {
			s.WriteString("  No observations recorded for this task.\n")
		} else {
			for _, o := range m.observations {
				s.WriteString(fmt.Sprintf("  • [%s] %s\n", o.CreatedAt.Format("2006-01-02 15:04:05"), o.Content))
			}
		}
		s.WriteString("\n" + helpStyle.Render("Press Esc or Enter to return to task list"))
		return s.String()
	}

	// 4. Guaranteed Non-Colliding Table Headers with Sort Direction & Selection Highlight
	fixedColumnsWidth := colMarkWidth + len(colGap) +
		colIDWidth + len(colGap) +
		colStatusWidth + len(colGap) +
		colPriWidth + len(colGap) +
		colProjectWidth + len(colGap) +
		colTagsWidth + len(colGap) +
		colDepsWidth + len(colGap) +
		colNotesWidth + len(colGap) +
		colCreatedWidth + len(colGap)

	titleWidth := max(20, m.width-fixedColumnsWidth-2)

	renderColHeader := func(colIdx int, baseTitle string, width int) string {
		title := baseTitle
		targetField := ColumnToSortField(colIdx)
		if m.sortOrder.Field == targetField {
			if m.sortOrder.Ascending {
				title += " ▲"
			} else {
				title += " ▼"
			}
		}
		if colIdx == m.selectedCol {
			return headerSelectedColStyle.Width(width).Render(truncate(title, width))
		}
		return headerCellStyle.Width(width).Render(truncate(title, width))
	}

	headerRow := strings.Join([]string{
		lipgloss.NewStyle().Width(colMarkWidth).Render(" "),
		renderColHeader(0, "ID", colIDWidth),
		renderColHeader(1, "Status", colStatusWidth),
		renderColHeader(2, "Pri", colPriWidth),
		renderColHeader(3, "Project", colProjectWidth),
		renderColHeader(4, "Tags", colTagsWidth),
		renderColHeader(5, "Deps", colDepsWidth),
		renderColHeader(6, "Notes", colNotesWidth),
		renderColHeader(7, "Created", colCreatedWidth),
		renderColHeader(8, "Title", titleWidth),
	}, colGap)

	s.WriteString(headerRow + "\n")
	s.WriteString(strings.Repeat("─", max(75, m.width-2)) + "\n")

	// 5. Task Rows with Strict Truncation, Crosshair Cell Highlight, and Viewport Scrolling
	if len(m.tasks) == 0 {
		s.WriteString("  No tasks found in this category.\n")
	} else {
		visibleRows := m.availableRows()
		endIdx := min(len(m.tasks), m.scrollOffset+visibleRows)

		for i := m.scrollOffset; i < endIdx; i++ {
			t := m.tasks[i]
			isRowSelected := i == m.cursor

			cursorMark := "  "
			if isRowSelected {
				cursorMark = "▸ "
			}

			statusStr := m.formatStatusText(t)
			prioStr := m.formatPriorityText(t.Priority)
			projectStr := truncate(t.Project, colProjectWidth)
			tagsStr := truncate(strings.Join(t.Tags, ","), colTagsWidth)

			var depList []string
			for _, d := range t.Dependencies {
				depList = append(depList, strconv.FormatInt(d, 10))
			}
			depsStr := truncate(strings.Join(depList, ","), colDepsWidth)

			// Requirement 4: Observation count column
			notesCount := 0
			if m.obsCounts != nil {
				notesCount = m.obsCounts[t.ID]
			}
			notesStr := strconv.Itoa(notesCount)

			// Requirement 5: Human-readable timestamp
			createdStr := t.CreatedAt.Local().Format("2006-01-02 15:04")
			titleStr := truncate(t.Title, titleWidth)

			cells := []string{
				strconv.FormatInt(t.ID, 10),
				statusStr,
				prioStr,
				projectStr,
				tagsStr,
				depsStr,
				notesStr,
				createdStr,
				titleStr,
			}
			widths := []int{
				colIDWidth,
				colStatusWidth,
				colPriWidth,
				colProjectWidth,
				colTagsWidth,
				colDepsWidth,
				colNotesWidth,
				colCreatedWidth,
				titleWidth,
			}

			var renderedCells []string
			renderedCells = append(renderedCells, lipgloss.NewStyle().Width(colMarkWidth).Render(cursorMark))

			for colIdx := 0; colIdx < TotalColumns; colIdx++ {
				cellVal := cells[colIdx]
				w := widths[colIdx]

				// Requirement 1: Visually select line and column intersection
				if isRowSelected && colIdx == m.selectedCol {
					renderedCells = append(renderedCells, selectedCellStyle.Width(w).Render(cellVal))
				} else {
					renderedCells = append(renderedCells, lipgloss.NewStyle().Width(w).Render(cellVal))
				}
			}

			rowLine := strings.Join(renderedCells, colGap)
			if isRowSelected {
				s.WriteString(selectedRowStyle.Render(rowLine) + "\n")
			} else {
				s.WriteString(rowLine + "\n")
			}
		}
	}

	// 6. Requirement 7: Readonly Detail Inspection Pane
	s.WriteString(m.renderDetailPane())

	// 7. Requirement 8: Command Mode Input or Status Line
	if m.viewMode == ModeCommand {
		s.WriteString(cmdPrompt.Render(":") + m.commandInput + "█\n")
	} else if m.statusMsg != "" {
		s.WriteString(helpStyle.Render("Status: ") + m.statusMsg + "\n")
	} else {
		s.WriteString(helpStyle.Render("Normal Mode • h/l: col • j/k: row • s: sort • /: search • :: cmd • ?: help") + "\n")
	}

	return s.String()
}

// Requirement 7: Readonly pane inspecting description and observations of selected task
func (m Model) renderDetailPane() string {
	paneHeight := m.previewPaneHeight()
	var sb strings.Builder

	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		sep := strings.Repeat("─", max(40, m.width-2))
		sb.WriteString(paneSubStyle.Render(sep) + "\n")
		sb.WriteString(paneSubStyle.Render("  (No task selected)\n"))
		return sb.String()
	}

	t := m.tasks[m.cursor]
	sepTitle := fmt.Sprintf("── Task #%d: %s ", t.ID, truncate(t.Title, 45))
	remainingLen := max(0, m.width-len(sepTitle)-2)
	sb.WriteString(paneBarHead.Render(sepTitle) + paneSubStyle.Render(strings.Repeat("─", remainingLen)) + "\n")

	// Metadata line
	var metaParts []string
	metaParts = append(metaParts, fmt.Sprintf("Status: %s", t.Status))
	metaParts = append(metaParts, fmt.Sprintf("Pri: %s", t.Priority))
	if t.Project != "" {
		metaParts = append(metaParts, fmt.Sprintf("Project: %s", t.Project))
	}
	if len(t.Tags) > 0 {
		metaParts = append(metaParts, fmt.Sprintf("Tags: [%s]", strings.Join(t.Tags, ", ")))
	}
	if len(t.Dependencies) > 0 {
		var depStrs []string
		for _, d := range t.Dependencies {
			depStrs = append(depStrs, strconv.FormatInt(d, 10))
		}
		metaParts = append(metaParts, fmt.Sprintf("Deps: #%s", strings.Join(depStrs, ", #")))
	}
	sb.WriteString(paneSubStyle.Render("  " + strings.Join(metaParts, " • ")) + "\n")

	// Description or blocker
	linesPrinted := 2
	if t.BlockerReason != "" {
		sb.WriteString(blockedStyle.Render("  Blocker: " + t.BlockerReason) + "\n")
		linesPrinted++
	}

	desc := strings.TrimSpace(t.Description)
	if desc != "" {
		firstLine := strings.Split(desc, "\n")[0]
		sb.WriteString(fmt.Sprintf("  Desc: %s\n", truncate(firstLine, max(20, m.width-12))))
		linesPrinted++
	}

	// Observations history
	if len(m.selectedTaskObs) == 0 {
		if linesPrinted < paneHeight {
			sb.WriteString(paneSubStyle.Render("  Notes: (None recorded)\n"))
			linesPrinted++
		}
	} else {
		for _, o := range m.selectedTaskObs {
			if linesPrinted >= paneHeight {
				break
			}
			noteLine := fmt.Sprintf("  • [%s] %s", o.CreatedAt.Format("15:04"), truncate(o.Content, max(20, m.width-18)))
			sb.WriteString(paneSubStyle.Render(noteLine) + "\n")
			linesPrinted++
		}
	}

	for linesPrinted < paneHeight {
		sb.WriteString("\n")
		linesPrinted++
	}

	return sb.String()
}

func (m Model) renderFullScreenHelp() string {
	rawHelpLines := []string{
		"================================================================================",
		"                       TaskManager TUI Controls & Help                          ",
		"================================================================================",
		"",
		"NAVIGATION & SELECTION (Vim-Style):",
		"  j, Down              Move row cursor down",
		"  k, Up                Move row cursor up",
		"  h, Left              Select column to the left",
		"  l, Right             Select column to the right",
		"  gg                   Jump to top of task list (first task)",
		"  G, End               Jump to bottom of task list (last task)",
		"  PgUp, PgDown         Scroll task list by page",
		"  Tab, Shift+Tab       Cycle category tabs (All, Ready, Pending, InProg, Blocked, Done)",
		"  1 - 6                Jump directly to category tab",
		"",
		"SORTING (Interactive Column Sorting):",
		"  s                    Sort by currently selected column.",
		"                       Pressing 's' again toggles sort direction (▲ / ▼).",
		"",
		"TASK ACTIONS:",
		"  a, c                 Create new task using $EDITOR (vim/nano)",
		"  e                    Edit selected task in $EDITOR (modifies fields and notes)",
		"  Enter                Start task (marks in_progress & active)",
		"  x                    Mark task completed (done)",
		"  b                    Block task (prompts for blocker reason)",
		"  v                    View task observation notes modal",
		"",
		"COMMAND MODE (Vim-Style ':' Commands):",
		"  :                    Enter command line mode",
		"  :q, :exit            Quit TaskManager",
		"  :add <title>         Create task with title",
		"  :start [id]          Start task (defaults to currently selected task)",
		"  :done [id]           Mark task completed",
		"  :block [id] <reason> Block task with reason",
		"  :log [id] <note>     Append note to task observation history",
		"  :tag [id] +tag -tag  Add or remove tags",
		"  :dep [id] +dep -dep  Add or remove dependencies",
		"  :del [id]            Delete / cancel task",
		"  :edit [id]           Open task in $EDITOR",
		"",
		"FILTERING & SEARCH:",
		"  /                    Filter by text, +tag, +READY, +BLOCKED, project:x, pri:H",
		"  Esc                  Clear filter / exit modals",
		"  ?                    Toggle this help window",
		"",
		"================================================================================",
		"            Use j/k or Up/Down to scroll • Esc/Enter/? to close                 ",
		"================================================================================",
	}

	visibleHeight := max(5, m.height-2)
	maxOffset := max(0, len(rawHelpLines)-visibleHeight)
	if m.helpScrollOffset > maxOffset {
		m.helpScrollOffset = maxOffset
	}

	endIdx := min(len(rawHelpLines), m.helpScrollOffset+visibleHeight)
	visibleLines := rawHelpLines[m.helpScrollOffset:endIdx]

	boxWidth := max(40, m.width-4)
	content := strings.Join(visibleLines, "\n")

	fullBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(boxWidth).
		Height(visibleHeight)

	return fullBoxStyle.Render(content)
}

func (m Model) countForTab(tabIdx int) int {
	criteria := ParseSearchQuery(m.searchQuery)
	cnt := 0
	for _, t := range m.allTasks {
		vtags := model.ComputeVirtualTags(t, m.allTasks, false)
		vtagMap := make(map[string]bool)
		for _, vt := range vtags {
			vtagMap[vt] = true
		}

		matchTab := false
		switch tabIdx {
		case 0:
			matchTab = true
		case 1:
			matchTab = vtagMap["READY"]
		case 2:
			matchTab = t.Status == model.StatusPending
		case 3:
			matchTab = t.Status == model.StatusInProgress
		case 4:
			matchTab = vtagMap["BLOCKED"]
		case 5:
			matchTab = t.Status == model.StatusCompleted
		}

		if matchTab && MatchesFilter(t, m.allTasks, false, criteria) {
			cnt++
		}
	}
	return cnt
}

func (m Model) formatStatusText(t *model.Task) string {
	rawStatus := string(t.Status)
	if t.Active {
		rawStatus = "in_prog*"
	}
	switch t.Status {
	case model.StatusInProgress:
		return inProgressStyle.Render(rawStatus)
	case model.StatusCompleted:
		return completedStyle.Render(rawStatus)
	case model.StatusBlocked:
		return blockedStyle.Render(rawStatus)
	case model.StatusPending:
		return pendingStyle.Render(rawStatus)
	case model.StatusCancelled:
		return cancelledStyle.Render(rawStatus)
	default:
		return rawStatus
	}
}

func (m Model) formatPriorityText(p model.TaskPriority) string {
	str := string(p)
	if str == "" {
		str = "M"
	}
	switch p {
	case model.PriorityCritical:
		return criticalPrioStyle.Render(str)
	case model.PriorityHigh:
		return highPrioStyle.Render(str)
	case model.PriorityMedium:
		return mediumPrioStyle.Render(str)
	case model.PriorityLow:
		return lowPrioStyle.Render(str)
	default:
		return str
	}
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) > maxLen {
		if maxLen > 3 {
			return string(runes[:maxLen-3]) + "..."
		}
		return string(runes[:maxLen])
	}
	return s
}

func Run(st *store.Store) error {
	p := tea.NewProgram(New(st), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
