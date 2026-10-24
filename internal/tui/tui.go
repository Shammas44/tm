package tui

import (
	"context"
	"fmt"
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

	tabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("238"))

	inProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	completedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	blockedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	pendingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type ViewMode int

const (
	ModeList ViewMode = iota
	ModeObservations
	ModeSearch
	ModeBlockInput
)

type Model struct {
	store        *store.Store
	tasks        []*model.Task
	allTasks     []*model.Task
	activeTab    int // 0: All, 1: Pending, 2: In Progress, 3: Blocked, 4: Completed
	cursor       int
	viewMode     ViewMode
	observations []*model.Observation
	searchQuery  string
	blockInput   string
	statusMsg    string
	width        int
	height       int
}

var tabs = []string{"All", "Pending", "In Progress", "Blocked", "Completed"}

func New(st *store.Store) Model {
	m := Model{
		store:     st,
		activeTab: 0,
		viewMode:  ModeList,
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
	var filtered []*model.Task
	for _, t := range m.allTasks {
		if m.searchQuery != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(m.searchQuery)) {
			continue
		}
		switch m.activeTab {
		case 1: // Pending
			if t.Status == model.StatusPending {
				filtered = append(filtered, t)
			}
		case 2: // In Progress
			if t.Status == model.StatusInProgress {
				filtered = append(filtered, t)
			}
		case 3: // Blocked
			if t.Status == model.StatusBlocked {
				filtered = append(filtered, t)
			}
		case 4: // Completed
			if t.Status == model.StatusCompleted {
				filtered = append(filtered, t)
			}
		default:
			filtered = append(filtered, t)
		}
	}
	m.tasks = filtered
	if m.cursor >= len(m.tasks) {
		m.cursor = max(0, len(m.tasks)-1)
	}
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
		return m, nil

	case tea.KeyMsg:
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

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
			}

		case "tab":
			m.activeTab = (m.activeTab + 1) % len(tabs)
			m.cursor = 0
			m.filterTasks()

		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(tabs)) % len(tabs)
			m.cursor = 0
			m.filterTasks()

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
	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("TaskManager TUI"))
	s.WriteString("  ")

	// Tabs
	for i, tab := range tabs {
		if i == m.activeTab {
			s.WriteString(activeTabStyle.Render(tab))
		} else {
			s.WriteString(tabStyle.Render(tab))
		}
		s.WriteString(" ")
	}
	s.WriteString("\n\n")

	// Search bar if searching
	if m.viewMode == ModeSearch {
		s.WriteString(fmt.Sprintf("Search: %s█\n\n", m.searchQuery))
	} else if m.searchQuery != "" {
		s.WriteString(fmt.Sprintf("Filtered by: '%s' (press / to change, esc to clear)\n\n", m.searchQuery))
	}

	// Block reason input prompt
	if m.viewMode == ModeBlockInput {
		t := m.tasks[m.cursor]
		s.WriteString(fmt.Sprintf("Enter block reason for #%d '%s': %s█\n(press Enter to confirm, Esc to cancel)\n\n",
			t.ID, t.Title, m.blockInput))
		return s.String()
	}

	// Observation modal view
	if m.viewMode == ModeObservations {
		t := m.tasks[m.cursor]
		s.WriteString(fmt.Sprintf("=== Observations for Task #%d: %s ===\n\n", t.ID, t.Title))
		if len(m.observations) == 0 {
			s.WriteString("  No observations recorded for this task.\n")
		} else {
			for _, o := range m.observations {
				s.WriteString(fmt.Sprintf("  • [%s] %s\n", o.CreatedAt.Format("15:04:05"), o.Content))
			}
		}
		s.WriteString("\n" + helpStyle.Render("Press Esc or Enter to return to task list"))
		return s.String()
	}

	// Task List Table
	if len(m.tasks) == 0 {
		s.WriteString("  No tasks found in this view.\n\n")
	} else {
		s.WriteString(fmt.Sprintf("  %-4s  %-12s  %-4s  %-12s  %-35s\n", "ID", "Status", "Pri", "Project", "Title"))
		s.WriteString("  " + strings.Repeat("-", 72) + "\n")

		for i, t := range m.tasks {
			cursorMark := " "
			if i == m.cursor {
				cursorMark = "▸"
			}

			statusStr := string(t.Status)
			if t.Active {
				statusStr = inProgressStyle.Render("in_prog*")
			} else {
				switch t.Status {
				case model.StatusCompleted:
					statusStr = completedStyle.Render(string(t.Status))
				case model.StatusBlocked:
					statusStr = blockedStyle.Render(string(t.Status))
				case model.StatusPending:
					statusStr = pendingStyle.Render(string(t.Status))
				}
			}

			row := fmt.Sprintf("%s %-4d  %-12s  %-4s  %-12s  %-35s",
				cursorMark, t.ID, statusStr, t.Priority, t.Project, t.Title)

			if i == m.cursor {
				s.WriteString(selectedRowStyle.Render(row) + "\n")
			} else {
				s.WriteString(row + "\n")
			}
		}
	}

	s.WriteString("\n")
	if m.statusMsg != "" {
		s.WriteString("Status: " + m.statusMsg + "\n")
	}

	// Footer help
	s.WriteString(helpStyle.Render("Controls: j/k: navigate • Tab: switch tab • Enter: start • x: complete • b: block • v: notes • /: search • q: quit"))

	return s.String()
}

func Run(st *store.Store) error {
	p := tea.NewProgram(New(st), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
