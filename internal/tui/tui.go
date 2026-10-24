package tui

import (
	"bufio"
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
			Foreground(lipgloss.Color("12")).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("25")).
			Padding(0, 1)

	headerCellStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	rowBlueBg    = lipgloss.Color("25") // Royal Blue for selected row
	colLighterBg = lipgloss.Color("33") // Lighter Blue for selected column

	headerSelectedColStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(colLighterBg)

	selectedRowCellStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(rowBlueBg)

	selectedColCellStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(colLighterBg)

	selectedGapStyle = lipgloss.NewStyle().
				Background(rowBlueBg)

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
	paneBarHead  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
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

func (m Model) topSectionOverhead() int {
	overhead := 4 // Title banner (2) + Table header & sep (2)
	if m.searchQuery != "" || m.viewMode == ModeSearch {
		overhead += 2
	}
	return overhead
}

func (m Model) availableRows() int {
	// Task list takes half of screen height
	halfTop := m.height / 2
	overhead := m.topSectionOverhead()
	rows := halfTop - overhead
	if rows < 2 {
		return 2
	}
	return rows
}

func (m Model) previewPaneHeight() int {
	// Task details takes the other half of screen height
	halfTop := m.height / 2
	halfBottom := m.height - halfTop
	// 1 line for details separator, 1 line for command/spacing footer
	p := halfBottom - 2
	if p < 3 {
		return 3
	}
	return p
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
			maxOffset := m.maxHelpScrollOffset()
			switch msg.String() {
			case "esc", "q", "?", "enter":
				m.viewMode = ModeList
				m.helpScrollOffset = 0
				return m, nil
			case "j", "down":
				if m.helpScrollOffset < maxOffset {
					m.helpScrollOffset++
				}
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
				m.helpScrollOffset = maxOffset
				return m, nil
			case "pgdown", "ctrl+d":
				step := max(1, m.helpVisibleHeight()/2)
				m.helpScrollOffset = min(maxOffset, m.helpScrollOffset+step)
				return m, nil
			case "pgup", "ctrl+u":
				step := max(1, m.helpVisibleHeight()/2)
				m.helpScrollOffset = max(0, m.helpScrollOffset-step)
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
					reason = strings.ReplaceAll(reason, "\\n", "\n")
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
				m.previewScroll = 0
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
				m.previewScroll = 0
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
				m.previewScroll = 0
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		// Scroll detail inspection pane up / down
		case "J", "ctrl+d":
			m.previewScroll += 2

		case "K", "ctrl+u":
			if m.previewScroll > 0 {
				m.previewScroll -= 2
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
			m.previewScroll = 0
			m.loadSelectedTaskObs()

		case "G", "end":
			if len(m.tasks) > 0 {
				m.cursor = len(m.tasks) - 1
				m.previewScroll = 0
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		case "pgup":
			m.cursor = max(0, m.cursor-m.availableRows())
			m.previewScroll = 0
			m.adjustScroll()
			m.loadSelectedTaskObs()

		case "pgdown":
			if len(m.tasks) > 0 {
				m.cursor = min(len(m.tasks)-1, m.cursor+m.availableRows())
				m.previewScroll = 0
				m.adjustScroll()
				m.loadSelectedTaskObs()
			}

		case "tab":
			m.activeTab = (m.activeTab + 1) % len(tabs)
			m.cursor = 0
			m.scrollOffset = 0
			m.previewScroll = 0
			m.filterTasks()
			m.loadSelectedTaskObs()

		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(tabs)) % len(tabs)
			m.cursor = 0
			m.scrollOffset = 0
			m.previewScroll = 0
			m.filterTasks()
			m.loadSelectedTaskObs()

		case "1", "2", "3", "4", "5", "6":
			tabIdx := int(msg.String()[0] - '1')
			if tabIdx < len(tabs) {
				m.activeTab = tabIdx
				m.cursor = 0
				m.scrollOffset = 0
				m.previewScroll = 0
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

		case "/": // Search
			m.viewMode = ModeSearch
			m.searchQuery = ""
		}
	}

	return m, nil
}

func (m Model) openCreateEditor() (Model, tea.Cmd) {
	tmpFile, err := os.CreateTemp("", "tm-create-*.yaml")
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
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("tm-edit-%d-*.yaml", task.ID))
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
		reason = strings.ReplaceAll(reason, "\\n", "\n")
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

	// Range & help shortcut info
	rangeInfo := ""
	if len(m.tasks) > 0 {
		visibleEnd := min(len(m.tasks), m.scrollOffset+m.availableRows())
		rangeInfo = fmt.Sprintf("[%d-%d of %d]  ?: help", m.scrollOffset+1, visibleEnd, len(m.tasks))
	} else {
		rangeInfo = "[0 tasks]  ?: help"
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
	visibleRows := m.availableRows()
	renderedRows := 0

	if len(m.tasks) == 0 {
		s.WriteString("  No tasks found in this category.\n")
		renderedRows = 1
	} else {
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

			rawStatus := string(t.Status)
			if t.Active {
				rawStatus = "in_prog*"
			}
			rawPrio := string(t.Priority)
			if rawPrio == "" {
				rawPrio = "M"
			}

			var cells []string
			if isRowSelected {
				cells = []string{
					strconv.FormatInt(t.ID, 10),
					rawStatus,
					rawPrio,
					projectStr,
					tagsStr,
					depsStr,
					notesStr,
					createdStr,
					titleStr,
				}
			} else {
				cells = []string{
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
			if isRowSelected {
				renderedCells = append(renderedCells, selectedRowCellStyle.Width(colMarkWidth).Render(cursorMark))
				for colIdx := 0; colIdx < TotalColumns; colIdx++ {
					cellVal := cells[colIdx]
					w := widths[colIdx]
					if colIdx == m.selectedCol {
						renderedCells = append(renderedCells, selectedColCellStyle.Width(w).Render(cellVal))
					} else {
						renderedCells = append(renderedCells, selectedRowCellStyle.Width(w).Render(cellVal))
					}
				}
				gap := selectedGapStyle.Render(colGap)
				rowLine := strings.Join(renderedCells, gap)
				s.WriteString(rowLine + "\n")
			} else {
				renderedCells = append(renderedCells, lipgloss.NewStyle().Width(colMarkWidth).Render(cursorMark))
				for colIdx := 0; colIdx < TotalColumns; colIdx++ {
					cellVal := cells[colIdx]
					w := widths[colIdx]
					renderedCells = append(renderedCells, lipgloss.NewStyle().Width(w).Render(cellVal))
				}
				rowLine := strings.Join(renderedCells, colGap)
				s.WriteString(rowLine + "\n")
			}
			renderedRows++
		}
	}

	// Pad unused rows so task list consistently occupies top half of screen height
	for r := renderedRows; r < visibleRows; r++ {
		s.WriteString("\n")
	}

	// 6. Requirement 7: Readonly Detail Inspection Pane (occupies bottom half of screen height)
	s.WriteString(m.renderDetailPane())

	// Command Mode Input or clean spacing line
	if m.viewMode == ModeCommand {
		s.WriteString(cmdPrompt.Render(":") + m.commandInput + "█\n")
	} else {
		s.WriteString("\n")
	}

	return s.String()
}

// Requirement 7: Readonly pane inspecting description and observations of selected task
func (m Model) renderDetailPane() string {
	paneHeight := m.previewPaneHeight()
	var allLines []string

	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		sep := strings.Repeat("─", max(40, m.width-2))
		var sb strings.Builder
		sb.WriteString(paneSubStyle.Render(sep) + "\n")
		sb.WriteString(paneSubStyle.Render("  (No task selected)\n"))
		for i := 2; i < paneHeight; i++ {
			sb.WriteString("\n")
		}
		return sb.String()
	}

	t := m.tasks[m.cursor]

	// 1. Blocker Reason
	hasBlockers := false
	if strings.TrimSpace(t.BlockerReason) != "" {
		hasBlockers = true
		allLines = append(allLines, blockedStyle.Render("Blocker Reason:"))
		reason := strings.TrimSpace(t.BlockerReason)
		reasonWrapped := wrapText(reason, max(30, m.width-6), "    ")
		for _, rl := range reasonWrapped {
			allLines = append(allLines, rl)
		}
	}

	if len(t.Dependencies) > 0 {
		var blockingDeps []string
		for _, depID := range t.Dependencies {
			var depTask *model.Task
			for _, other := range m.allTasks {
				if other.ID == depID {
					depTask = other
					break
				}
			}
			if depTask != nil && depTask.Status != model.StatusCompleted {
				blockingDeps = append(blockingDeps, fmt.Sprintf("#%d %q (%s)", depTask.ID, truncate(depTask.Title, 25), depTask.Status))
			} else if depTask == nil {
				blockingDeps = append(blockingDeps, fmt.Sprintf("#%d (unknown)", depID))
			}
		}
		if len(blockingDeps) > 0 {
			if hasBlockers {
				allLines = append(allLines, "")
			}
			hasBlockers = true
			allLines = append(allLines, blockedStyle.Render("Blocked by Prerequisite:"))
			for _, bd := range blockingDeps {
				allLines = append(allLines, "    "+bd)
			}
		} else if t.Status == model.StatusPending && !hasBlockers {
			allLines = append(allLines, completedStyle.Render("✓ All prerequisites completed (Ready to start)"))
		}
	}

	// 2. Description
	desc := strings.TrimSpace(t.Description)
	if desc != "" {
		if hasBlockers {
			allLines = append(allLines, "")
		}
		allLines = append(allLines, paneBarHead.Render("Description:"))
		descWrapped := wrapText(desc, max(30, m.width-6), "    ")
		for _, dl := range descWrapped {
			allLines = append(allLines, dl)
		}
	}

	// 3. Observations & Notes
	if len(m.selectedTaskObs) > 0 {
		if hasBlockers || desc != "" {
			allLines = append(allLines, "")
		}
		allLines = append(allLines, paneBarHead.Render(fmt.Sprintf("Observation (%d recorded):", len(m.selectedTaskObs))))
		for obsIdx, o := range m.selectedTaskObs {
			if obsIdx > 0 {
				allLines = append(allLines, "") // empty line separator between observations
			}
			headerLine := fmt.Sprintf("- [%s] (#%d)", o.CreatedAt.Local().Format("2006-01-02 15:04"), o.ID)
			allLines = append(allLines, paneSubStyle.Render(headerLine))

			contentLines := strings.Split(strings.TrimSpace(o.Content), "\n")
			for _, rawLine := range contentLines {
				rawLine = strings.TrimSpace(rawLine)
				if rawLine == "" {
					allLines = append(allLines, "")
					continue
				}
				wrapped := wrapText(rawLine, max(30, m.width-6), "  ")
				for _, wl := range wrapped {
					allLines = append(allLines, wl)
				}
			}
		}
	} else if !hasBlockers && desc == "" {
		allLines = append(allLines, paneSubStyle.Render("(No description, blockers, or observations recorded)"))
	}

	// Calculate scrolling
	maxScroll := max(0, len(allLines)-paneHeight)
	if m.previewScroll > maxScroll {
		m.previewScroll = maxScroll
	}
	if m.previewScroll < 0 {
		m.previewScroll = 0
	}

	// Pane title bar with scroll indicator if content overflows
	scrollHint := ""
	if len(allLines) > paneHeight {
		scrollHint = fmt.Sprintf(" [%d-%d of %d • J/K to scroll] ",
			m.previewScroll+1,
			min(len(allLines), m.previewScroll+paneHeight),
			len(allLines),
		)
	}

	sepTitle := fmt.Sprintf("── Details #%d%s ", t.ID, scrollHint)
	remainingLen := max(0, m.width-len(sepTitle)-2)
	headerBar := paneBarHead.Render(sepTitle) + paneSubStyle.Render(strings.Repeat("─", remainingLen))

	var sb strings.Builder
	sb.WriteString(headerBar + "\n")

	endIdx := min(len(allLines), m.previewScroll+paneHeight)
	linesRendered := 0
	if m.previewScroll < len(allLines) {
		for i := m.previewScroll; i < endIdx; i++ {
			sb.WriteString(allLines[i] + "\n")
			linesRendered++
		}
	}
	for linesRendered < paneHeight {
		sb.WriteString("\n")
		linesRendered++
	}

	return sb.String()
}

func (m Model) helpVisibleHeight() int {
	return max(6, m.height-4)
}

func (m Model) helpContentLines() []string {
	keyWidth := 26
	descWidth := max(25, m.width-keyWidth-4)
	lineWidth := min(m.width, keyWidth+descWidth+2)

	hKey := func(k string) string {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Width(keyWidth).Render(k)
	}
	hDesc := func(d string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(descWidth).Render(truncate(d, descWidth))
	}
	hSec := func(t string) string {
		barLen := max(0, lineWidth-len(t)-5)
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")).Render("── " + t + " " + strings.Repeat("─", barLen))
	}

	return []string{
		hSec("NAVIGATION"),
		hKey("  j / k, ↓ / ↑") + hDesc("Navigate rows (tasks)"),
		hKey("  h / l, ← / →") + hDesc("Select column left / right"),
		hKey("  gg / G") + hDesc("Jump to first (top) / last (bottom) task"),
		hKey("  PgUp / PgDown") + hDesc("Scroll task list by page"),
		hKey("  Tab / Shift+Tab") + hDesc("Cycle category tabs"),
		hKey("  1 - 6") + hDesc("Jump directly to category tab"),
		"",
		hSec("INSPECTION & SCROLLING"),
		hKey("  J / K") + hDesc("Scroll task detail pane down / up"),
		hKey("  Ctrl+D / Ctrl+U") + hDesc("Half-page scroll detail pane down / up"),
		"",
		hSec("COLUMN SORTING"),
		hKey("  s") + hDesc("Sort by selected column (press again to reverse ▲/▼)"),
		"",
		hSec("TASK ACTIONS"),
		hKey("  Enter") + hDesc("Start task (mark in_progress & active)"),
		hKey("  x") + hDesc("Complete task (mark done)"),
		hKey("  b") + hDesc("Block task (prompts for blocker reason)"),
		hKey("  a, c") + hDesc("Create new task in $EDITOR"),
		hKey("  e") + hDesc("Edit task & past observations in $EDITOR"),
		"",
		hSec("VIM COMMAND MODE (:)"),
		hKey("  :add <title>") + hDesc("Create new task"),
		hKey("  :start [id]") + hDesc("Start task (defaults to selected)"),
		hKey("  :done [id]") + hDesc("Complete task"),
		hKey("  :block [id] <r>") + hDesc("Block task with reason"),
		hKey("  :log [id] <note>") + hDesc("Append note/observation"),
		hKey("  :tag [id] +/-tag") + hDesc("Add or remove tags"),
		hKey("  :dep [id] +/-id") + hDesc("Add or remove dependencies"),
		hKey("  :edit [id]") + hDesc("Edit task in $EDITOR"),
		hKey("  :del [id]") + hDesc("Delete task"),
		hKey("  :q, :wq, :exit") + hDesc("Quit TaskManager"),
		"",
		hSec("SEARCH & FILTER"),
		hKey("  /") + hDesc("Search text, +tag, +READY, +BLOCKED, pri:H, project:x"),
		hKey("  Esc") + hDesc("Clear search / close modal"),
	}
}

func (m Model) maxHelpScrollOffset() int {
	totalLines := len(m.helpContentLines())
	visHeight := m.helpVisibleHeight()
	maxOffset := totalLines - visHeight
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m Model) renderFullScreenHelp() string {
	lines := m.helpContentLines()
	visibleHeight := m.helpVisibleHeight()

	maxOffset := m.maxHelpScrollOffset()
	scrollOffset := m.helpScrollOffset
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}

	endIdx := min(len(lines), scrollOffset+visibleHeight)
	visibleLines := lines[scrollOffset:endIdx]

	var sb strings.Builder
	// Header bar: No border
	sb.WriteString(titleStyle.Render("TaskManager • Controls & Commands Reference"))
	sb.WriteString(" ")
	scrollInfo := fmt.Sprintf("[%d-%d of %d]  ?: close", scrollOffset+1, endIdx, len(lines))
	sb.WriteString(helpStyle.Render(scrollInfo))
	sb.WriteString("\n\n")

	sb.WriteString(strings.Join(visibleLines, "\n"))
	sb.WriteString("\n\n")

	// Footer bar: Controls hint
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Press Esc, Enter, q, or ? to return to tasks • j/k to scroll • g/G for top/bottom\n"))

	return sb.String()
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

func wrapText(text string, width int, indent string) []string {
	if width <= 10 {
		width = 40
	}
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		rawLine := scanner.Text()
		if len(rawLine) == 0 {
			result = append(result, "")
			continue
		}
		words := strings.Fields(rawLine)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		var curLine strings.Builder
		curLine.WriteString(indent)
		curLen := len(indent)

		for _, w := range words {
			if curLen+len(w)+1 > width {
				result = append(result, curLine.String())
				curLine.Reset()
				curLine.WriteString(indent)
				curLine.WriteString(w)
				curLen = len(indent) + len(w)
			} else {
				if curLen > len(indent) {
					curLine.WriteString(" ")
					curLen++
				}
				curLine.WriteString(w)
				curLen += len(w)
			}
		}
		if curLine.Len() > len(indent) {
			result = append(result, curLine.String())
		}
	}
	return result
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
