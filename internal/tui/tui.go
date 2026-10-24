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

const (
	AppName = "TaskManager"
	Version = "v0.1.0"
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

	topAppStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("25")).
			Padding(0, 1)

	topViewStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("14"))

	topCatStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("11"))

	topHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Padding(0, 1)

	topSepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	bottomRecordsStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15"))

	bottomFilterStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14"))

	bottomSortStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	bottomStatusMsgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11"))

	bottomHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	searchPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("14"))

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

	// Session Table Column Widths
	colSessIDWidth       = 10
	colSessStatusWidth   = 13
	colSessAgentWidth    = 18
	colSessStartedWidth  = 16
	colSessDurationWidth = 10
	colSessTasksWidth    = 5
	colSessNotesWidth    = 5
)

type ActiveTable int

const (
	TableTasks ActiveTable = iota
	TableSessions
)

type ViewMode int

const (
	ModeList ViewMode = iota
	ModeCommand
	ModeSearch
	ModeBlockInput
	ModeHelp
	ModeDeleteConfirm
)

type editorDoneMsg struct {
	mode      string // "create", "edit", or "session"
	taskID    int64
	sessionID string
	tempPath  string
	err       error
}

type Model struct {
	store       *store.Store
	activeTable ActiveTable

	// Task table state
	tasks        []*model.Task
	allTasks     []*model.Task
	obsCounts    map[int64]int
	activeTab    int // 0: All, 1: Ready, 2: Pending, 3: In Progress, 4: Blocked, 5: Completed
	cursor       int // Selected row index
	selectedCol  int // Selected column index (0 to TotalColumns-1)
	scrollOffset int // Top visible row index
	sortOrder    SortOrder

	// Session table state
	sessions            []*model.Session
	allSessions         []*model.Session
	sessionObsCounts    map[string]int
	sessionTaskCounts   map[string]int
	sessionActiveTab    int // 0: All, 1: Active, 2: Completed, 3: Interrupted
	sessionCursor       int // Selected row index in session table
	sessionSelectedCol  int // Selected column index (0 to TotalSessionColumns-1)
	sessionScrollOffset int // Top visible row index in session table
	sessionSortOrder    SessionSortOrder

	helpScrollOffset   int
	viewMode           ViewMode
	taskSearchQuery    string
	sessionSearchQuery string
	commandInput       string // For Vim command mode ':'
	blockInput       string
	statusMsg        string
	deleteTargetTask *model.Task
	deleteBlockedMsg string
	lastKey          rune
	lastKeyPressTime time.Time
	width            int
	height           int
}

var tabs = []string{"All", "Ready", "Pending", "In Progress", "Blocked", "Completed"}
var sessionTabs = []string{"All", "Today", "Active", "Completed", "Interrupted", "Handoff"}

func New(st *store.Store) Model {
	m := Model{
		store:              st,
		activeTable:        TableTasks,
		activeTab:          0,
		sessionActiveTab:   0,
		viewMode:           ModeList,
		selectedCol:        0,
		sessionSelectedCol: 0,
		sortOrder:          SortOrder{Field: SortByID, Ascending: true},
		sessionSortOrder:   SessionSortOrder{Field: SessionSortByStarted, Ascending: false},
		width:              80,
		height:             24,
	}
	m.reloadTasks()
	m.reloadSessions()
	return m
}

func (m Model) activeSearchQuery() string {
	if m.activeTable == TableSessions {
		return m.sessionSearchQuery
	}
	return m.taskSearchQuery
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
}

func (m *Model) filterTasks() {
	criteria := ParseSearchQuery(m.taskSearchQuery)
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

func (m *Model) reloadSessions() {
	ctx := context.Background()
	sessions, err := m.store.ListSessions(ctx, 200)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error loading sessions: %v", err)
		return
	}
	m.allSessions = sessions

	obsCounts := make(map[string]int)
	taskCounts := make(map[string]int)
	for _, sess := range sessions {
		obsList, err := m.store.ListObservations(ctx, store.ObservationFilter{SessionID: sess.ID})
		if err == nil {
			obsCounts[sess.ID] = len(obsList)
			touched := make(map[int64]bool)
			if sess.ActiveTaskID != nil {
				touched[*sess.ActiveTaskID] = true
			}
			for _, o := range obsList {
				if o.TaskID != nil {
					touched[*o.TaskID] = true
				}
			}
			taskCounts[sess.ID] = len(touched)
		}
	}
	m.sessionObsCounts = obsCounts
	m.sessionTaskCounts = taskCounts

	m.filterSessions()
}

func (m *Model) filterSessions() {
	var filtered []*model.Session
	now := time.Now()

	for _, s := range m.allSessions {
		switch m.sessionActiveTab {
		case 1: // Today
			if s.StartedAt.Local().YearDay() != now.YearDay() || s.StartedAt.Local().Year() != now.Year() {
				continue
			}
		case 2: // Active
			if s.Status != model.SessionStatusActive {
				continue
			}
		case 3: // Completed
			if s.Status != model.SessionStatusCompleted {
				continue
			}
		case 4: // Interrupted
			if s.Status != model.SessionStatusInterrupted {
				continue
			}
		case 5: // Handoff
			if !hasSessionHandoff(s) {
				continue
			}
		}

		if MatchesSessionFilter(s, m.sessionSearchQuery) {
			filtered = append(filtered, s)
		}
	}

	SortSessions(filtered, m.sessionSortOrder, m.sessionObsCounts, m.sessionTaskCounts)
	m.sessions = filtered

	if m.sessionCursor >= len(m.sessions) {
		m.sessionCursor = max(0, len(m.sessions)-1)
	}
	m.adjustSessionScroll()
}

func (m *Model) adjustSessionScroll() {
	visibleRows := m.availableRows()
	if visibleRows <= 0 {
		return
	}
	if m.sessionCursor < m.sessionScrollOffset {
		m.sessionScrollOffset = m.sessionCursor
	}
	if m.sessionCursor >= m.sessionScrollOffset+visibleRows {
		m.sessionScrollOffset = m.sessionCursor - visibleRows + 1
	}
	if m.sessionScrollOffset < 0 {
		m.sessionScrollOffset = 0
	}
}

func (m Model) countForSessionTab(tabIdx int) int {
	cnt := 0
	now := time.Now()
	for _, s := range m.allSessions {
		match := false
		switch tabIdx {
		case 0: // All
			match = true
		case 1: // Today
			match = s.StartedAt.Local().YearDay() == now.YearDay() && s.StartedAt.Local().Year() == now.Year()
		case 2: // Active
			match = (s.Status == model.SessionStatusActive)
		case 3: // Completed
			match = (s.Status == model.SessionStatusCompleted)
		case 4: // Interrupted
			match = (s.Status == model.SessionStatusInterrupted)
		case 5: // Handoff
			match = hasSessionHandoff(s)
		}
		if match && MatchesSessionFilter(s, m.sessionSearchQuery) {
			cnt++
		}
	}
	return cnt
}

func hasSessionHandoff(s *model.Session) bool {
	if s.Handoff != nil {
		if str, ok := s.Handoff.(string); ok {
			return strings.TrimSpace(str) != ""
		}
		if m, ok := s.Handoff.(map[string]any); ok {
			return len(m) > 0
		}
		return true
	}
	return strings.TrimSpace(s.Summary) != ""
}

func (m Model) topSectionOverhead() int {
	return 4 // Top status bar (1) + Spacing (1) + Table header (1) + Table sep (1)
}

func (m Model) availableRows() int {
	// Full screen height available minus top overhead (4) and bottom status line (1)
	overhead := m.topSectionOverhead() + 1
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
		m.adjustSessionScroll()
		return m, nil

	case editorDoneMsg:
		if msg.err != nil {
			_ = os.Remove(msg.tempPath)
			m.statusMsg = fmt.Sprintf("Editor exited with error: %v", msg.err)
			return m, nil
		}

		if msg.mode == "session" {
			_ = os.Remove(msg.tempPath)
			shortID := msg.sessionID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			m.statusMsg = fmt.Sprintf("Closed inspection for session %s", shortID)
			return m, nil
		}

		contentBytes, readErr := os.ReadFile(msg.tempPath)
		_ = os.Remove(msg.tempPath)

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
				Title:        parsed.Title,
				Status:       parsed.Status,
				Priority:     parsed.Priority,
				Project:      parsed.Project,
				Description:  parsed.Description,
				Blocker:      parsed.Blocker,
				Tags:         parsed.Tags,
				Dependencies: parsed.Dependencies,
			}
			if strings.TrimSpace(parsed.Blocker) != "" {
				newTask.Status = model.StatusBlocked
			}
			created, err := m.store.CreateTask(ctx, newTask)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Failed to create task: %v", err)
				return m, nil
			}
			if created.Status == model.StatusInProgress {
				_ = m.store.SetActiveTask(ctx, created.ID)
			}

			// Add observations defined in notes
			for _, content := range parsed.ExistingNotes {
				if strings.TrimSpace(content) != "" {
					_, _ = m.store.AddObservation(ctx, sessionID, &created.ID, strings.TrimSpace(content))
				}
			}
			for _, newNote := range parsed.NewNotes {
				if strings.TrimSpace(newNote) != "" {
					_, _ = m.store.AddObservation(ctx, sessionID, &created.ID, strings.TrimSpace(newNote))
				}
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
			existing.Blocker = parsed.Blocker
			existing.Tags = parsed.Tags
			existing.Dependencies = parsed.Dependencies
			if strings.TrimSpace(parsed.Blocker) != "" {
				existing.Status = model.StatusBlocked
			}

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
			origIDs := make(map[int64]bool)
			for _, o := range origObs {
				origIDs[o.ID] = true
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

			// Add newly added notes
			for id, content := range parsed.ExistingNotes {
				if !origIDs[id] && strings.TrimSpace(content) != "" {
					_, _ = m.store.AddObservation(ctx, sessionID, &existing.ID, strings.TrimSpace(content))
				}
			}
			for _, newNote := range parsed.NewNotes {
				if strings.TrimSpace(newNote) != "" {
					_, _ = m.store.AddObservation(ctx, sessionID, &existing.ID, strings.TrimSpace(newNote))
				}
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
				if m.activeTable == TableSessions {
					m.sessionSearchQuery = ""
					m.filterSessions()
				} else {
					m.taskSearchQuery = ""
					m.filterTasks()
				}
				m.viewMode = ModeList
				return m, nil
			case "enter":
				m.viewMode = ModeList
				return m, nil
			case "backspace":
				if m.activeTable == TableSessions {
					if len(m.sessionSearchQuery) > 0 {
						m.sessionSearchQuery = m.sessionSearchQuery[:len(m.sessionSearchQuery)-1]
						m.filterSessions()
					}
				} else {
					if len(m.taskSearchQuery) > 0 {
						m.taskSearchQuery = m.taskSearchQuery[:len(m.taskSearchQuery)-1]
						m.filterTasks()
					}
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					if m.activeTable == TableSessions {
						m.sessionSearchQuery += msg.String()
						m.filterSessions()
					} else {
						m.taskSearchQuery += msg.String()
						m.filterTasks()
					}
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

		if m.viewMode == ModeDeleteConfirm {
			if m.deleteBlockedMsg != "" {
				m.viewMode = ModeList
				m.deleteBlockedMsg = ""
				return m, nil
			}
			switch msg.String() {
			case "y", "Y", "enter":
				if m.deleteTargetTask != nil {
					_ = m.store.DeleteTaskPermanently(ctx, m.deleteTargetTask.ID)
					m.deleteTargetTask = nil
					m.viewMode = ModeList
					m.reloadTasks()
				}
				return m, nil
			case "n", "N", "esc", "q":
				m.deleteTargetTask = nil
				m.viewMode = ModeList
				return m, nil
			}
			return m, nil
		}

		// ModeList keys
		// Requirement 3: Vim 'gg' and 'G' navigation
		if msg.String() == "g" {
			now := time.Now()
			if m.lastKey == 'g' && now.Sub(m.lastKeyPressTime) < 800*time.Millisecond {
				if m.activeTable == TableSessions {
					m.sessionCursor = 0
					m.sessionScrollOffset = 0
					m.lastKey = 0
					m.adjustSessionScroll()
				} else {
					m.cursor = 0
					m.scrollOffset = 0
					m.lastKey = 0
					m.adjustScroll()
				}
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

		// Switch tables (Tasks <-> Sessions)
		case "t", "S":
			if m.activeTable == TableTasks {
				m.activeTable = TableSessions
			} else {
				m.activeTable = TableTasks
			}

		// Requirement 1: Vertical line navigation
		case "up", "k":
			if m.activeTable == TableSessions {
				if m.sessionCursor > 0 {
					m.sessionCursor--
					m.adjustSessionScroll()
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
					m.adjustScroll()
				}
			}

		case "down", "j":
			if m.activeTable == TableSessions {
				if m.sessionCursor < len(m.sessions)-1 {
					m.sessionCursor++
					m.adjustSessionScroll()
				}
			} else {
				if m.cursor < len(m.tasks)-1 {
					m.cursor++
					m.adjustScroll()
				}
			}

		// Half-page scroll
		case "ctrl+d", "J":
			if m.activeTable == TableSessions {
				if len(m.sessions) > 0 {
					step := max(1, m.availableRows()/2)
					m.sessionCursor = min(len(m.sessions)-1, m.sessionCursor+step)
					m.adjustSessionScroll()
				}
			} else {
				if len(m.tasks) > 0 {
					step := max(1, m.availableRows()/2)
					m.cursor = min(len(m.tasks)-1, m.cursor+step)
					m.adjustScroll()
				}
			}

		case "ctrl+u", "K":
			if m.activeTable == TableSessions {
				step := max(1, m.availableRows()/2)
				m.sessionCursor = max(0, m.sessionCursor-step)
				m.adjustSessionScroll()
			} else {
				step := max(1, m.availableRows()/2)
				m.cursor = max(0, m.cursor-step)
				m.adjustScroll()
			}

		// Requirement 1: Horizontal column navigation
		case "left", "h":
			if m.activeTable == TableSessions {
				if m.sessionSelectedCol > 0 {
					m.sessionSelectedCol--
				}
			} else {
				if m.selectedCol > 0 {
					m.selectedCol--
				}
			}

		case "right", "l":
			if m.activeTable == TableSessions {
				if m.sessionSelectedCol < TotalSessionColumns-1 {
					m.sessionSelectedCol++
				}
			} else {
				if m.selectedCol < TotalColumns-1 {
					m.selectedCol++
				}
			}

		case "home":
			if m.activeTable == TableSessions {
				m.sessionCursor = 0
				m.sessionScrollOffset = 0
			} else {
				m.cursor = 0
				m.scrollOffset = 0
			}

		case "G", "end":
			if m.activeTable == TableSessions {
				if len(m.sessions) > 0 {
					m.sessionCursor = len(m.sessions) - 1
					m.adjustSessionScroll()
				}
			} else {
				if len(m.tasks) > 0 {
					m.cursor = len(m.tasks) - 1
					m.adjustScroll()
				}
			}

		case "pgup":
			if m.activeTable == TableSessions {
				m.sessionCursor = max(0, m.sessionCursor-m.availableRows())
				m.adjustSessionScroll()
			} else {
				m.cursor = max(0, m.cursor-m.availableRows())
				m.adjustScroll()
			}

		case "pgdown":
			if m.activeTable == TableSessions {
				if len(m.sessions) > 0 {
					m.sessionCursor = min(len(m.sessions)-1, m.sessionCursor+m.availableRows())
					m.adjustSessionScroll()
				}
			} else {
				if len(m.tasks) > 0 {
					m.cursor = min(len(m.tasks)-1, m.cursor+m.availableRows())
					m.adjustScroll()
				}
			}

		case "tab":
			if m.activeTable == TableSessions {
				m.sessionActiveTab = (m.sessionActiveTab + 1) % len(sessionTabs)
				m.sessionCursor = 0
				m.sessionScrollOffset = 0
				m.filterSessions()
			} else {
				m.activeTab = (m.activeTab + 1) % len(tabs)
				m.cursor = 0
				m.scrollOffset = 0
				m.filterTasks()
			}

		case "shift+tab":
			if m.activeTable == TableSessions {
				m.sessionActiveTab = (m.sessionActiveTab - 1 + len(sessionTabs)) % len(sessionTabs)
				m.sessionCursor = 0
				m.sessionScrollOffset = 0
				m.filterSessions()
			} else {
				m.activeTab = (m.activeTab - 1 + len(tabs)) % len(tabs)
				m.cursor = 0
				m.scrollOffset = 0
				m.filterTasks()
			}

		case "1", "2", "3", "4", "5", "6":
			tabIdx := int(msg.String()[0] - '1')
			if m.activeTable == TableSessions {
				if tabIdx < len(sessionTabs) {
					m.sessionActiveTab = tabIdx
					m.sessionCursor = 0
					m.sessionScrollOffset = 0
					m.filterSessions()
				}
			} else {
				if tabIdx < len(tabs) {
					m.activeTab = tabIdx
					m.cursor = 0
					m.scrollOffset = 0
					m.filterTasks()
				}
			}

		// Sort by selected column, toggle direction if pressed again
		case "s":
			if m.activeTable == TableSessions {
				targetField := ColumnToSessionSortField(m.sessionSelectedCol)
				if m.sessionSortOrder.Field == targetField {
					m.sessionSortOrder.Ascending = !m.sessionSortOrder.Ascending
				} else {
					m.sessionSortOrder.Field = targetField
					m.sessionSortOrder.Ascending = DefaultSessionAscendingForField(targetField)
				}
				SortSessions(m.sessions, m.sessionSortOrder, m.sessionObsCounts, m.sessionTaskCounts)
				m.statusMsg = fmt.Sprintf("Sorted by %s", m.sessionSortOrder.Label())
			} else {
				targetField := ColumnToSortField(m.selectedCol)
				if m.sortOrder.Field == targetField {
					m.sortOrder.Ascending = !m.sortOrder.Ascending
				} else {
					m.sortOrder.Field = targetField
					m.sortOrder.Ascending = DefaultAscendingForField(targetField)
				}
				SortTasks(m.tasks, m.sortOrder, m.obsCounts)
				m.statusMsg = fmt.Sprintf("Sorted by %s", m.sortOrder.Label())
			}

		case "a", "c": // Add / Create task using $EDITOR
			if m.activeTable == TableSessions {
				m.statusMsg = "Task creation is only available in Tasks view (press 't')"
				return m, nil
			}
			return m.openCreateEditor()

		case "e", "v": // Edit task or inspect session using $EDITOR
			if m.activeTable == TableSessions {
				if len(m.sessions) > 0 && m.sessionCursor < len(m.sessions) {
					return m.openSessionEditor(m.sessions[m.sessionCursor].ID)
				}
				return m, nil
			}
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				return m.openEditEditor(m.tasks[m.cursor].ID)
			}

		case "enter": // Start task or inspect session
			if m.activeTable == TableSessions {
				if len(m.sessions) > 0 && m.sessionCursor < len(m.sessions) {
					return m.openSessionEditor(m.sessions[m.sessionCursor].ID)
				}
				return m, nil
			}
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				t := m.tasks[m.cursor]
				if err := m.store.SetActiveTask(ctx, t.ID); err == nil {
					m.statusMsg = fmt.Sprintf("Started task #%d (%s)", t.ID, t.Title)
					m.reloadTasks()
				}
			}

		case "x": // Mark done
			if m.activeTable == TableSessions {
				m.statusMsg = "Marking done is only available in Tasks view"
				return m, nil
			}
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				t := m.tasks[m.cursor]
				if err := m.store.UpdateTaskStatus(ctx, t.ID, model.StatusCompleted, ""); err == nil {
					m.statusMsg = fmt.Sprintf("Completed task #%d (%s)", t.ID, t.Title)
					m.reloadTasks()
				}
			}

		case "b": // Block task
			if m.activeTable == TableSessions {
				m.statusMsg = "Blocking is only available in Tasks view"
				return m, nil
			}
			if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
				m.blockInput = ""
				m.viewMode = ModeBlockInput
			}

		case "d": // Delete task permanently with confirmation & dependency check
			if m.activeTable == TableSessions {
				m.statusMsg = "Deleting is only available in Tasks view"
				return m, nil
			}
			if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
				return m, nil
			}
			t := m.tasks[m.cursor]

			// Ensure no other task depends on this one
			var dependentTasks []string
			for _, other := range m.allTasks {
				if other.ID == t.ID {
					continue
				}
				for _, depID := range other.Dependencies {
					if depID == t.ID {
						dependentTasks = append(dependentTasks, fmt.Sprintf("#%d %q", other.ID, truncate(other.Title, 25)))
						break
					}
				}
			}

			if len(dependentTasks) > 0 {
				m.viewMode = ModeDeleteConfirm
				m.deleteBlockedMsg = fmt.Sprintf("Cannot delete task #%d: prerequisite for task(s): %s.\nRemove dependencies first.\n\nPress any key to return.", t.ID, strings.Join(dependentTasks, ", "))
				m.deleteTargetTask = nil
				return m, nil
			}

			m.viewMode = ModeDeleteConfirm
			m.deleteTargetTask = t
			m.deleteBlockedMsg = ""
			return m, nil

		case "/": // Search
			m.viewMode = ModeSearch
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

	case "tasks", "task":
		m.activeTable = TableTasks
		return m, nil

	case "sessions", "session":
		m.activeTable = TableSessions
		return m, nil

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

	case "view", "open":
		if m.activeTable == TableSessions {
			if len(m.sessions) > 0 && m.sessionCursor < len(m.sessions) {
				return m.openSessionEditor(m.sessions[m.sessionCursor].ID)
			}
			m.statusMsg = "No session selected."
			return m, nil
		}
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

	case "edit":
		if m.activeTable == TableSessions {
			if len(m.sessions) > 0 && m.sessionCursor < len(m.sessions) {
				return m.openSessionEditor(m.sessions[m.sessionCursor].ID)
			}
			m.statusMsg = "No session selected."
			return m, nil
		}
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

		// Ensure no other task depends on this one
		var dependentTasks []string
		for _, other := range m.allTasks {
			if other.ID == id {
				continue
			}
			for _, depID := range other.Dependencies {
				if depID == id {
					dependentTasks = append(dependentTasks, fmt.Sprintf("#%d %q", other.ID, truncate(other.Title, 25)))
					break
				}
			}
		}

		if len(dependentTasks) > 0 {
			m.viewMode = ModeDeleteConfirm
			m.deleteBlockedMsg = fmt.Sprintf("Cannot delete task #%d: prerequisite for task(s): %s.\nRemove dependencies first.\n\nPress any key to return.", id, strings.Join(dependentTasks, ", "))
			m.deleteTargetTask = nil
			return m, nil
		}

		var targetTask *model.Task
		for _, other := range m.allTasks {
			if other.ID == id {
				targetTask = other
				break
			}
		}
		if targetTask == nil {
			targetTask, _ = m.store.GetTask(ctx, id)
		}
		if targetTask == nil {
			m.statusMsg = fmt.Sprintf("Task #%d not found", id)
			return m, nil
		}

		m.viewMode = ModeDeleteConfirm
		m.deleteTargetTask = targetTask
		m.deleteBlockedMsg = ""
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

	// 1. Top status line
	s.WriteString(m.renderTopStatusLine())
	s.WriteString("\n\n")

	// 2. Error modal if delete blocked
	if m.viewMode == ModeDeleteConfirm && m.deleteBlockedMsg != "" {
		s.WriteString(titleStyle.Render("TaskManager • Delete Task"))
		s.WriteString("\n\n")
		s.WriteString(blockedStyle.Render("Cannot Delete Task:\n"))
		s.WriteString(m.deleteBlockedMsg + "\n")
		return s.String()
	}

	// 3. Table content
	if m.activeTable == TableTasks {
		m.renderTaskTable(&s)
	} else {
		m.renderSessionTable(&s)
	}

	// 4. Bottom status line (records, filter, command/search prompt)
	s.WriteString(m.renderBottomStatusLine())

	return s.String()
}

func (m Model) renderTopStatusLine() string {
	viewName := "Tasks"
	catName := tabs[m.activeTab]
	catIdx := m.activeTab + 1
	catTotal := len(tabs)

	if m.activeTable == TableSessions {
		viewName = "Sessions"
		catName = sessionTabs[m.sessionActiveTab]
		catIdx = m.sessionActiveTab + 1
		catTotal = len(sessionTabs)
	}

	appStr := topAppStyle.Render(fmt.Sprintf("%s %s", AppName, Version))
	sep := topSepStyle.Render("│")

	viewText := fmt.Sprintf("View: %s (switch: t)", viewName)
	if m.width < 95 {
		viewText = fmt.Sprintf("View: %s (t)", viewName)
	}
	viewStr := topViewStyle.Render(viewText)

	catText := fmt.Sprintf("Category: %s [%d/%d]", catName, catIdx, catTotal)
	if m.width < 80 {
		catText = fmt.Sprintf("Cat: %s", catName)
	}
	catStr := topCatStyle.Render(catText)

	helpStr := topHelpStyle.Render("?: Help")

	left := fmt.Sprintf("%s %s %s %s %s", appStr, sep, viewStr, sep, catStr)
	leftWidth := lipgloss.Width(left)
	helpWidth := lipgloss.Width(helpStr)

	gap := m.width - leftWidth - helpWidth - 1
	if gap < 1 {
		return truncate(left+" "+helpStr, m.width-1)
	}

	return left + strings.Repeat(" ", gap) + helpStr
}

func (m Model) renderBottomStatusLine() string {
	switch m.viewMode {
	case ModeSearch:
		query := m.activeSearchQuery()
		prompt := searchPromptStyle.Render("/") + query + "█"
		hint := bottomHintStyle.Render("Esc: clear • Enter: confirm")
		pW := lipgloss.Width(prompt)
		hW := lipgloss.Width(hint)
		gap := m.width - pW - hW - 1
		if gap < 1 {
			return truncate(prompt, m.width-1)
		}
		return prompt + strings.Repeat(" ", gap) + hint

	case ModeCommand:
		prompt := cmdPrompt.Render(":") + m.commandInput + "█"
		hint := bottomHintStyle.Render("Esc: cancel • Enter: run")
		pW := lipgloss.Width(prompt)
		hW := lipgloss.Width(hint)
		gap := m.width - pW - hW - 1
		if gap < 1 {
			return truncate(prompt, m.width-1)
		}
		return prompt + strings.Repeat(" ", gap) + hint

	case ModeBlockInput:
		prompt := blockedStyle.Render("Block reason: ") + m.blockInput + "█"
		hint := bottomHintStyle.Render("Esc: cancel • Enter: save")
		pW := lipgloss.Width(prompt)
		hW := lipgloss.Width(hint)
		gap := m.width - pW - hW - 1
		if gap < 1 {
			return truncate(prompt, m.width-1)
		}
		return prompt + strings.Repeat(" ", gap) + hint

	case ModeDeleteConfirm:
		if m.deleteTargetTask != nil {
			prompt := criticalPrioStyle.Render(fmt.Sprintf("Delete task #%d '%s'? (y/n): █", m.deleteTargetTask.ID, truncate(m.deleteTargetTask.Title, 25)))
			hint := bottomHintStyle.Render("y: confirm • n/Esc: cancel")
			pW := lipgloss.Width(prompt)
			hW := lipgloss.Width(hint)
			gap := m.width - pW - hW - 1
			if gap < 1 {
				return truncate(prompt, m.width-1)
			}
			return prompt + strings.Repeat(" ", gap) + hint
		}
	}

	// Normal list mode: panel-specific records, filter, and sort order
	var recordsStr string
	var filterStr string
	var sortStr string

	if m.activeTable == TableTasks {
		if len(m.tasks) == len(m.allTasks) {
			recordsStr = fmt.Sprintf("Records: %d", len(m.tasks))
		} else {
			recordsStr = fmt.Sprintf("Records: %d of %d", len(m.tasks), len(m.allTasks))
		}
		if m.taskSearchQuery == "" {
			filterStr = "Filter: None"
		} else {
			filterStr = fmt.Sprintf("Filter: '%s'", m.taskSearchQuery)
		}
		sortStr = "Sort: " + m.sortOrder.Label()
	} else {
		if len(m.sessions) == len(m.allSessions) {
			recordsStr = fmt.Sprintf("Records: %d", len(m.sessions))
		} else {
			recordsStr = fmt.Sprintf("Records: %d of %d", len(m.sessions), len(m.allSessions))
		}
		if m.sessionSearchQuery == "" {
			filterStr = "Filter: None"
		} else {
			filterStr = fmt.Sprintf("Filter: '%s'", m.sessionSearchQuery)
		}
		sortStr = "Sort: " + m.sessionSortOrder.Label()
	}

	recStyled := bottomRecordsStyle.Render(recordsStr)
	fltStyled := bottomFilterStyle.Render(filterStr)
	sortStyled := bottomSortStyle.Render(sortStr)
	sep := topSepStyle.Render("│")

	left := fmt.Sprintf("%s %s %s %s %s", recStyled, sep, fltStyled, sep, sortStyled)
	if m.statusMsg != "" {
		left += " " + sep + " " + bottomStatusMsgStyle.Render(m.statusMsg)
	}

	hint := bottomHintStyle.Render("/ filter • : cmd")
	leftW := lipgloss.Width(left)
	hintW := lipgloss.Width(hint)

	gap := m.width - leftW - hintW - 1
	if gap < 1 {
		return truncate(left, m.width-1)
	}

	return left + strings.Repeat(" ", gap) + hint
}

func (m Model) renderTaskTable(s *strings.Builder) {
	// Guaranteed Non-Colliding Table Headers with Sort Direction & Selection Highlight
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

	// Task Rows with Strict Truncation, Crosshair Cell Highlight, and Viewport Scrolling
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

			notesCount := 0
			if m.obsCounts != nil {
				notesCount = m.obsCounts[t.ID]
			}
			notesStr := strconv.Itoa(notesCount)

			createdStr := t.CreatedAt.Local().Format("2006-01-02 15:04")
			titleStr := truncate(t.Title, titleWidth)

			rawStatus := strings.ToUpper(string(t.Status))
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

	for r := renderedRows; r < visibleRows; r++ {
		s.WriteString("\n")
	}
}

func (m Model) renderSessionTable(s *strings.Builder) {
	fixedColumnsWidth := colMarkWidth + len(colGap) +
		colSessIDWidth + len(colGap) +
		colSessStatusWidth + len(colGap) +
		colSessAgentWidth + len(colGap) +
		colSessStartedWidth + len(colGap) +
		colSessDurationWidth + len(colGap) +
		colSessTasksWidth + len(colGap) +
		colSessNotesWidth + len(colGap)

	handoffWidth := max(20, m.width-fixedColumnsWidth-2)

	renderColHeader := func(colIdx int, baseTitle string, width int) string {
		title := baseTitle
		targetField := ColumnToSessionSortField(colIdx)
		if m.sessionSortOrder.Field == targetField {
			if m.sessionSortOrder.Ascending {
				title += " ▲"
			} else {
				title += " ▼"
			}
		}
		if colIdx == m.sessionSelectedCol {
			return headerSelectedColStyle.Width(width).Render(truncate(title, width))
		}
		return headerCellStyle.Width(width).Render(truncate(title, width))
	}

	headerRow := strings.Join([]string{
		lipgloss.NewStyle().Width(colMarkWidth).Render(" "),
		renderColHeader(0, "Session", colSessIDWidth),
		renderColHeader(1, "Status", colSessStatusWidth),
		renderColHeader(2, "Agent", colSessAgentWidth),
		renderColHeader(3, "Started", colSessStartedWidth),
		renderColHeader(4, "Duration", colSessDurationWidth),
		renderColHeader(5, "Tasks", colSessTasksWidth),
		renderColHeader(6, "Notes", colSessNotesWidth),
		renderColHeader(7, "Handoff / Checkpoint", handoffWidth),
	}, colGap)

	s.WriteString(headerRow + "\n")
	s.WriteString(strings.Repeat("─", max(75, m.width-2)) + "\n")

	visibleRows := m.availableRows()
	renderedRows := 0

	if len(m.sessions) == 0 {
		s.WriteString("  No sessions found in this category.\n")
		renderedRows = 1
	} else {
		endIdx := min(len(m.sessions), m.sessionScrollOffset+visibleRows)

		for i := m.sessionScrollOffset; i < endIdx; i++ {
			sess := m.sessions[i]
			isRowSelected := i == m.sessionCursor

			cursorMark := "  "
			if isRowSelected {
				cursorMark = "▸ "
			}

			shortID := sess.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			idStr := truncate(shortID, colSessIDWidth)
			statusStr := formatSessionStatusText(sess.Status)
			rawStatus := strings.ToUpper(string(sess.Status))
			agentStr := truncate(sess.AgentID, colSessAgentWidth)
			startedStr := sess.StartedAt.Local().Format("2006-01-02 15:04")
			durStr := truncate(FormatSessionDuration(sess), colSessDurationWidth)
			tasksCount := m.sessionTaskCounts[sess.ID]
			tasksStr := strconv.Itoa(tasksCount)
			notesCount := m.sessionObsCounts[sess.ID]
			notesStr := strconv.Itoa(notesCount)

			handoffText := cleanOneLineHandoff(extractHandoffString(sess))
			handoffStr := truncate(handoffText, handoffWidth)

			var cells []string
			if isRowSelected {
				cells = []string{
					idStr,
					rawStatus,
					agentStr,
					startedStr,
					durStr,
					tasksStr,
					notesStr,
					handoffStr,
				}
			} else {
				cells = []string{
					idStr,
					statusStr,
					agentStr,
					startedStr,
					durStr,
					tasksStr,
					notesStr,
					handoffStr,
				}
			}
			widths := []int{
				colSessIDWidth,
				colSessStatusWidth,
				colSessAgentWidth,
				colSessStartedWidth,
				colSessDurationWidth,
				colSessTasksWidth,
				colSessNotesWidth,
				handoffWidth,
			}

			var renderedCells []string
			if isRowSelected {
				renderedCells = append(renderedCells, selectedRowCellStyle.Width(colMarkWidth).Render(cursorMark))
				for colIdx := 0; colIdx < TotalSessionColumns; colIdx++ {
					cellVal := cells[colIdx]
					w := widths[colIdx]
					if colIdx == m.sessionSelectedCol {
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
				for colIdx := 0; colIdx < TotalSessionColumns; colIdx++ {
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

	for r := renderedRows; r < visibleRows; r++ {
		s.WriteString("\n")
	}
}

func cleanOneLineHandoff(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

func formatSessionStatusText(status model.SessionStatus) string {
	raw := strings.ToUpper(string(status))
	switch status {
	case model.SessionStatusActive:
		return inProgressStyle.Render(raw)
	case model.SessionStatusCompleted:
		return completedStyle.Render(raw)
	case model.SessionStatusInterrupted:
		return blockedStyle.Render(raw)
	default:
		return raw
	}
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
		hSec("VIEW & NAVIGATION"),
		hKey("  t / S") + hDesc("Switch between Tasks and Sessions tables"),
		hKey("  j / k, ↓ / ↑") + hDesc("Navigate rows (tasks / sessions)"),
		hKey("  h / l, ← / →") + hDesc("Select column left / right"),
		hKey("  gg / G") + hDesc("Jump to first (top) / last (bottom)"),
		hKey("  Ctrl+D / Ctrl+U") + hDesc("Scroll list half-page down / up"),
		hKey("  PgUp / PgDown") + hDesc("Scroll list by page"),
		hKey("  Tab / Shift+Tab") + hDesc("Cycle category filter tabs"),
		hKey("  1 - 6") + hDesc("Jump directly to category tab"),
		"",
		hSec("COLUMN SORTING"),
		hKey("  s") + hDesc("Sort by selected column (press again to reverse ▲/▼)"),
		"",
		hSec("TASK ACTIONS (Tasks View)"),
		hKey("  Enter") + hDesc("Start task (mark in_progress & active)"),
		hKey("  x") + hDesc("Complete task (mark done)"),
		hKey("  b") + hDesc("Block task (prompts for blocker)"),
		hKey("  d") + hDesc("Delete task permanently (with confirmation)"),
		hKey("  a, c") + hDesc("Create new task in $EDITOR"),
		hKey("  e") + hDesc("Edit task in $EDITOR"),
		"",
		hSec("SESSION ACTIONS (Sessions View)"),
		hKey("  Enter / e / v") + hDesc("Inspect full session YAML in $EDITOR (read-only)"),
		"",
		hSec("VIM COMMAND MODE (:)"),
		hKey("  :tasks / :sessions") + hDesc("Switch active table"),
		hKey("  :add <title>") + hDesc("Create new task"),
		hKey("  :start [id]") + hDesc("Start task (defaults to selected)"),
		hKey("  :done [id]") + hDesc("Complete task"),
		hKey("  :block [id] <r>") + hDesc("Block task with blocker"),
		hKey("  :log [id] <note>") + hDesc("Append note/observation"),
		hKey("  :tag [id] +/-tag") + hDesc("Add or remove tags"),
		hKey("  :dep [id] +/-id") + hDesc("Add or remove dependencies"),
		hKey("  :edit [id]") + hDesc("Edit task or session in $EDITOR"),
		hKey("  :del [id]") + hDesc("Delete task"),
		hKey("  :q, :wq, :exit") + hDesc("Quit TaskManager"),
		"",
		hSec("SEARCH & FILTER"),
		hKey("  /") + hDesc("Search text, tags, status, handoff, checkpoint"),
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
	criteria := ParseSearchQuery(m.taskSearchQuery)
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
	rawStatus := strings.ToUpper(string(t.Status))
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
