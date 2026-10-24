package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"taskmanager/internal/model"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	inProgressStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // Yellow
			Bold(true)

	completedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")) // Green

	blockedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // Red
			Bold(true)

	pendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")) // Cyan

	cancelledStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // Gray

	criticalPrioStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")). // Red
				Bold(true)

	highPrioStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")) // Orange

	mediumPrioStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")) // Blue

	lowPrioStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // Gray
)

type Formatter struct {
	ForceJSON bool
	Writer    io.Writer
}

func New(forceJSON bool, writer io.Writer) *Formatter {
	if writer == nil {
		writer = os.Stdout
	}
	return &Formatter{
		ForceJSON: forceJSON,
		Writer:    writer,
	}
}

func (f *Formatter) ShouldUseJSON() bool {
	if f.ForceJSON {
		return true
	}
	if file, ok := f.Writer.(*os.File); ok {
		return !isatty.IsTerminal(file.Fd()) && !isatty.IsCygwinTerminal(file.Fd())
	}
	return false
}

func (f *Formatter) PrintJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f.Writer, string(data))
	return err
}

func (f *Formatter) PrintTasks(tasks []*model.Task) error {
	if f.ShouldUseJSON() {
		return f.PrintJSON(tasks)
	}

	if len(tasks) == 0 {
		fmt.Fprintln(f.Writer, "No tasks found.")
		return nil
	}

	fmt.Fprintf(f.Writer, "%-4s  %-12s  %-4s  %-12s  %-35s  %-6s\n",
		headerStyle.Render("ID"),
		headerStyle.Render("Status"),
		headerStyle.Render("Pri"),
		headerStyle.Render("Project"),
		headerStyle.Render("Title"),
		headerStyle.Render("Age"),
	)
	fmt.Fprintln(f.Writer, strings.Repeat("-", 80))

	for _, t := range tasks {
		statusStr := formatStatus(t.Status)
		prioStr := formatPriority(t.Priority)
		title := t.Title
		if len(title) > 35 {
			title = title[:32] + "..."
		}
		age := formatAge(t.CreatedAt)

		fmt.Fprintf(f.Writer, "%-4d  %-12s  %-4s  %-12s  %-35s  %-6s\n",
			t.ID,
			statusStr,
			prioStr,
			t.Project,
			title,
			age,
		)
	}
	return nil
}

func (f *Formatter) PrintTask(t *model.Task) error {
	return f.PrintTaskDetails(t, nil, nil)
}

func (f *Formatter) PrintTaskDetails(t *model.Task, allTasks []*model.Task, obs []*model.Observation) error {
	if f.ShouldUseJSON() {
		type RichTask struct {
			*model.Task
			VirtualTags  []string             `json:"virtual_tags,omitempty"`
			Observations []*model.Observation `json:"observations,omitempty"`
		}
		var vtags []string
		if allTasks != nil {
			vtags = model.ComputeVirtualTags(t, allTasks, len(obs) > 0)
		}
		return f.PrintJSON(RichTask{
			Task:         t,
			VirtualTags:  vtags,
			Observations: obs,
		})
	}

	fmt.Fprintf(f.Writer, "Task #%d: %s\n", t.ID, headerStyle.Render(t.Title))
	fmt.Fprintf(f.Writer, "  Status:       %s", formatStatus(t.Status))
	if t.Active {
		fmt.Fprintf(f.Writer, " %s", inProgressStyle.Render("(active)"))
	}
	fmt.Fprintln(f.Writer)

	fmt.Fprintf(f.Writer, "  Priority:     %s\n", formatPriority(t.Priority))
	if t.Project != "" {
		fmt.Fprintf(f.Writer, "  Project:      %s\n", t.Project)
	}
	if len(t.Tags) > 0 {
		fmt.Fprintf(f.Writer, "  Tags:         %s\n", strings.Join(t.Tags, ", "))
	}
	if allTasks != nil {
		vtags := model.ComputeVirtualTags(t, allTasks, len(obs) > 0)
		if len(vtags) > 0 {
			fmt.Fprintf(f.Writer, "  Virtual Tags: %s\n", strings.Join(vtags, ", "))
		}
	}
	if len(t.Dependencies) > 0 {
		var depStrs []string
		for _, depID := range t.Dependencies {
			var depStatus model.TaskStatus = "unknown"
			for _, other := range allTasks {
				if other.ID == depID {
					depStatus = other.Status
					break
				}
			}
			depStrs = append(depStrs, fmt.Sprintf("#%d (%s)", depID, depStatus))
		}
		fmt.Fprintf(f.Writer, "  Dependencies: %s\n", strings.Join(depStrs, ", "))
	}
	if t.BlockerReason != "" {
		fmt.Fprintf(f.Writer, "  Block Reason: %s\n", blockedStyle.Render(t.BlockerReason))
	}
	if t.Description != "" {
		fmt.Fprintf(f.Writer, "  Description:  %s\n", t.Description)
	}
	fmt.Fprintf(f.Writer, "  Created:      %s (%s ago)\n", t.CreatedAt.Local().Format("2006-01-02 15:04:05"), formatAge(t.CreatedAt))
	fmt.Fprintf(f.Writer, "  Updated:      %s (%s ago)\n", t.UpdatedAt.Local().Format("2006-01-02 15:04:05"), formatAge(t.UpdatedAt))
	if t.CompletedAt != nil {
		fmt.Fprintf(f.Writer, "  Completed:    %s\n", t.CompletedAt.Local().Format("2006-01-02 15:04:05"))
	}

	if len(obs) > 0 {
		fmt.Fprintf(f.Writer, "\nObservations (%d recorded):\n", len(obs))
		for _, o := range obs {
			fmt.Fprintf(f.Writer, "  • [%s] (#%d) %s\n",
				o.CreatedAt.Local().Format("2006-01-02 15:04:05"),
				o.ID,
				o.Content,
			)
		}
	}

	return nil
}

func (f *Formatter) PrintObservations(obs []*model.Observation) error {
	if f.ShouldUseJSON() {
		return f.PrintJSON(obs)
	}

	if len(obs) == 0 {
		fmt.Fprintln(f.Writer, "No observations found.")
		return nil
	}

	fmt.Fprintf(f.Writer, "%-5s  %-6s  %-6s  %-55s\n",
		headerStyle.Render("ID"),
		headerStyle.Render("Task#"),
		headerStyle.Render("Age"),
		headerStyle.Render("Content"),
	)
	fmt.Fprintln(f.Writer, strings.Repeat("-", 80))

	for _, o := range obs {
		taskStr := "-"
		if o.TaskID != nil {
			taskStr = fmt.Sprintf("#%d", *o.TaskID)
		}
		content := o.Content
		if len(content) > 55 {
			content = content[:52] + "..."
		}
		age := formatAge(o.CreatedAt)

		fmt.Fprintf(f.Writer, "%-5d  %-6s  %-6s  %-55s\n",
			o.ID,
			taskStr,
			age,
			content,
		)
	}
	return nil
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

func formatStatus(s model.TaskStatus) string {
	switch s {
	case model.StatusInProgress:
		return inProgressStyle.Render(string(s))
	case model.StatusCompleted:
		return completedStyle.Render(string(s))
	case model.StatusBlocked:
		return blockedStyle.Render(string(s))
	case model.StatusPending:
		return pendingStyle.Render(string(s))
	case model.StatusCancelled:
		return cancelledStyle.Render(string(s))
	default:
		return string(s)
	}
}

func formatPriority(p model.TaskPriority) string {
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
