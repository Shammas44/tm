package tui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

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

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("238"))

	inProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	completedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	blockedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	pendingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	cancelledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	criticalPrioStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	highPrioStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	mediumPrioStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	lowPrioStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Column Widths with guaranteed 2-character gap separation
	colGap          = "  "
	colMarkWidth    = 2
	colIDWidth      = 4
	colStatusWidth  = 11
	colPriWidth     = 3
	colProjectWidth = 10
	colTagsWidth    = 12
	colDepsWidth    = 6
	colCreatedWidth = 16 // Human-readable timestamp e.g. "2026-10-24 14:15"
)

type ViewMode int

const (
	ModeList ViewMode = iota
	ModeObservations
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
	activeTab        int // 0: All, 1: Ready, 2: Pending, 3: In Progress, 4: Blocked, 5: Completed
	cursor           int
	scrollOffset     int
	helpScrollOffset int
	sortOrder        SortOrder
	viewMode         ViewMode
	observations     []*model.Observation
	searchQuery      string
	blockInput       string
	statusMsg        string
	width            int
	height           int
}

var tabs = []string{"All", "Ready", "Pending", "In Progress", "Blocked", "Completed"}

func New(st *store.Store) Model {
	m := Model{
		store:     st,
		activeTab: 0,
		viewMode:  ModeList,
		sortOrder: SortOrder{Field: SortByID, Ascending: true},
		width:     80,
		height:    24,
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
	m.filterTasks()
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

	SortTasks(filtered, m.sortOrder)
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

func (m Model) availableRows() int {
	rows := m.height - 7
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
					// User deleted the line from editor
					_ = m.store.DeleteObservation(ctx, o.ID)
				}
			}

			// Record new note if provided
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
			case "esc", "q", "h", "?", "enter":
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
				return m, nil
			case "enter":
				m.viewMode = ModeList
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.filterTasks()
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					m.filterTasks()
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
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "h", "?": // Help modal
			m.viewMode = ModeHelp
			m.helpScrollOffset = 0
			return m, nil

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}

		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
				m.adjustScroll()
			}

		case "g", "home":
			m.cursor = 0
			m.scrollOffset = 0

		case "G", "end":
			if len(m.tasks) > 0 {
				m.cursor = len(m.tasks) - 1
				m.adjustScroll()
			}

		case "pgup":
			m.cursor = max(0, m.cursor-m.availableRows())
			m.adjustScroll()

		case "pgdown":
			if len(m.tasks) > 0 {
				m.cursor = min(len(m.tasks)-1, m.cursor+m.availableRows())
				m.adjustScroll()
			}

		case "tab":
			m.activeTab = (m.activeTab + 1) % len(tabs)
			m.cursor = 0
			m.scrollOffset = 0
			m.filterTasks()

		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(tabs)) % len(tabs)
			m.cursor = 0
			m.scrollOffset = 0
			m.filterTasks()

		case "1", "2", "3", "4", "5", "6":
			tabIdx := int(msg.String()[0] - '1')
			if tabIdx < len(tabs) {
				m.activeTab = tabIdx
				m.cursor = 0
				m.scrollOffset = 0
				m.filterTasks()
			}

		case "s": // Cycle sort column
			m.sortOrder.CycleNext()
			SortTasks(m.tasks, m.sortOrder)
			m.statusMsg = fmt.Sprintf("Sorted by %s", m.sortOrder.Label())

		case "a", "c": // Add / Create task using $EDITOR
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

		case "e": // Edit selected task using $EDITOR
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				t := m.tasks[m.cursor]
				obs, _ := m.store.ListObservations(ctx, store.ObservationFilter{TaskID: &t.ID})
				tmpFile, err := os.CreateTemp("", fmt.Sprintf("tm-edit-%d-*.txt", t.ID))
				if err != nil {
					m.statusMsg = fmt.Sprintf("Failed to create temporary editor file: %v", err)
					return m, nil
				}
				tmpl := FormatEditTaskTemplate(t, m.allTasks, obs)
				_, _ = tmpFile.WriteString(tmpl)
				_ = tmpFile.Close()

				cmd := ResolveEditorCommand(tmpFile.Name())
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return editorDoneMsg{
						mode:     "edit",
						taskID:   t.ID,
						tempPath: tmpFile.Name(),
						err:      err,
					}
				})
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

		case "v": // View observations
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

func (m Model) View() string {
	// Full window help modal
	if m.viewMode == ModeHelp {
		return m.renderFullScreenHelp()
	}

	var s strings.Builder

	// 1. Header: Display ONLY the currently selected category tab
	s.WriteString(titleStyle.Render("TaskManager"))
	s.WriteString(" ")

	currentTabName := tabs[m.activeTab]
	currentTabCount := m.countForTab(m.activeTab)
	selectedTabBanner := fmt.Sprintf("Category: %s (%d tasks)", currentTabName, currentTabCount)
	s.WriteString(activeTabStyle.Render(selectedTabBanner))
	s.WriteString(" ")

	// Range and sort info
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

	// 4. Guaranteed Non-Colliding Table Headers
	// Fixed widths + 2-space gaps between columns
	fixedColumnsWidth := colMarkWidth + len(colGap) +
		colIDWidth + len(colGap) +
		colStatusWidth + len(colGap) +
		colPriWidth + len(colGap) +
		colProjectWidth + len(colGap) +
		colTagsWidth + len(colGap) +
		colDepsWidth + len(colGap) +
		colCreatedWidth + len(colGap)

	titleWidth := max(20, m.width-fixedColumnsWidth-2)

	headerRow := strings.Join([]string{
		lipgloss.NewStyle().Width(colMarkWidth).Render(" "),
		headerCellStyle.Width(colIDWidth).Render("ID"),
		headerCellStyle.Width(colStatusWidth).Render("Status"),
		headerCellStyle.Width(colPriWidth).Render("Pri"),
		headerCellStyle.Width(colProjectWidth).Render("Project"),
		headerCellStyle.Width(colTagsWidth).Render("Tags"),
		headerCellStyle.Width(colDepsWidth).Render("Deps"),
		headerCellStyle.Width(colCreatedWidth).Render("Created"),
		headerCellStyle.Width(titleWidth).Render("Title"),
	}, colGap)

	s.WriteString(headerRow + "\n")
	s.WriteString(strings.Repeat("-", max(75, m.width-2)) + "\n")

	// 5. Task Rows with Strict Truncation and Viewport Scrolling
	if len(m.tasks) == 0 {
		s.WriteString("  No tasks found in this category.\n")
	} else {
		visibleRows := m.availableRows()
		endIdx := min(len(m.tasks), m.scrollOffset+visibleRows)

		for i := m.scrollOffset; i < endIdx; i++ {
			t := m.tasks[i]
			isSelected := i == m.cursor

			cursorMark := "  "
			if isSelected {
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

			// Requirement 5: Human readable timestamp e.g. "2026-10-24 14:15"
			createdStr := t.CreatedAt.Local().Format("2006-01-02 15:04")
			titleStr := truncate(t.Title, titleWidth)

			rowLine := strings.Join([]string{
				lipgloss.NewStyle().Width(colMarkWidth).Render(cursorMark),
				lipgloss.NewStyle().Width(colIDWidth).Render(strconv.FormatInt(t.ID, 10)),
				lipgloss.NewStyle().Width(colStatusWidth).Render(statusStr),
				lipgloss.NewStyle().Width(colPriWidth).Render(prioStr),
				lipgloss.NewStyle().Width(colProjectWidth).Render(projectStr),
				lipgloss.NewStyle().Width(colTagsWidth).Render(tagsStr),
				lipgloss.NewStyle().Width(colDepsWidth).Render(depsStr),
				lipgloss.NewStyle().Width(colCreatedWidth).Render(createdStr),
				lipgloss.NewStyle().Width(titleWidth).Render(titleStr),
			}, colGap)

			if isSelected {
				s.WriteString(selectedRowStyle.Render(rowLine) + "\n")
			} else {
				s.WriteString(rowLine + "\n")
			}
		}
	}

	// 6. Status line & Controls info hidden from main view (Requirement 3)
	s.WriteString("\n")
	if m.statusMsg != "" {
		s.WriteString("Status: " + m.statusMsg)
	}

	return s.String()
}

// Requirement 4: Full window control popup with vertical scrolling
func (m Model) renderFullScreenHelp() string {
	rawHelpLines := []string{
		"================================================================================",
		"                       TaskManager TUI Controls & Help                          ",
		"================================================================================",
		"",
		"NAVIGATION & SCROLLING:",
		"  j, Down              Move cursor down",
		"  k, Up                Move cursor up",
		"  g, Home              Jump to top of task list",
		"  G, End               Jump to bottom of task list",
		"  PgUp, PgDown         Scroll task list by page",
		"  Tab                  Next category tab",
		"  Shift+Tab            Previous category tab",
		"  1 - 6                Jump directly to category (All, Ready, Pending, InProg, Blocked, Done)",
		"",
		"TASK EDITING & ACTIONS (vit-style):",
		"  a, c                 Create new task using $EDITOR (vim/nano)",
		"  e                    Edit task in $EDITOR (modifies fields and observation history)",
		"  Enter                Start task (marks in_progress & active)",
		"  x                    Mark task completed (done)",
		"  b                    Block task (prompts for blocker reason)",
		"  v                    View task observation notes modal",
		"",
		"OBSERVATION HISTORY EDITING (in editor):",
		"  • In the editor (e), existing notes appear as 'Note <id>: <content>'.",
		"  • Edit the text of any line to update that observation.",
		"  • Delete a note line (e.g. 'dd' in vim) or clear its text to delete that note.",
		"  • Write under 'New Note:' to append a new observation.",
		"",
		"FILTERING & SORTING:",
		"  /                    Search / filter by text, +tag, +READY, +BLOCKED, project:x, pri:H",
		"  s                    Cycle column sort (ID, Priority, Created, Status, Project, Title)",
		"",
		"WINDOW CONTROLS:",
		"  h, ?                 Toggle this help window",
		"  Esc, Enter, q        Close this help window and return to task list",
		"  Ctrl+C               Quit TaskManager TUI",
		"",
		"================================================================================",
		"                 Use j/k or Up/Down to scroll • Esc/Enter/h to close            ",
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
	count := 0
	for _, t := range m.allTasks {
		vtags := model.ComputeVirtualTags(t, m.allTasks, false)
		vtagMap := make(map[string]bool)
		for _, vt := range vtags {
			vtagMap[vt] = true
		}
		switch tabIdx {
		case 0: // All
			count++
		case 1: // Ready
			if vtagMap["READY"] {
				count++
			}
		case 2: // Pending
			if t.Status == model.StatusPending {
				count++
			}
		case 3: // In Progress
			if t.Status == model.StatusInProgress {
				count++
			}
		case 4: // Blocked
			if vtagMap["BLOCKED"] {
				count++
			}
		case 5: // Completed
			if t.Status == model.StatusCompleted {
				count++
			}
		}
	}
	return count
}

func (m Model) formatStatusText(t *model.Task) string {
	if t.Active {
		return inProgressStyle.Render("in_prog*")
	}
	switch t.Status {
	case model.StatusInProgress:
		return inProgressStyle.Render("in_progress")
	case model.StatusCompleted:
		return completedStyle.Render("completed")
	case model.StatusBlocked:
		return blockedStyle.Render("blocked")
	case model.StatusPending:
		return pendingStyle.Render("pending")
	case model.StatusCancelled:
		return cancelledStyle.Render("cancelled")
	default:
		return string(t.Status)
	}
}

func (m Model) formatPriorityText(p model.TaskPriority) string {
	switch p {
	case model.PriorityCritical:
		return criticalPrioStyle.Render(string(p))
	case model.PriorityHigh:
		return highPrioStyle.Render(string(p))
	case model.PriorityMedium:
		return mediumPrioStyle.Render(string(p))
	case model.PriorityLow:
		return lowPrioStyle.Render(string(p))
	default:
		return string(p)
	}
}

// Requirement 6: Strict truncation with "..."
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
