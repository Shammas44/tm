package tui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"taskmanager/internal/model"
)

type ParsedTask struct {
	Title         string
	Status        model.TaskStatus
	Priority      model.TaskPriority
	Project       string
	BlockerReason string
	Description   string
}

func FormatNewTaskTemplate(defaultProject string) string {
	var sb strings.Builder
	sb.WriteString("# TaskManager: Create New Task\n")
	sb.WriteString("# Lines starting with '#' are comments and will be ignored.\n")
	sb.WriteString("# Save and close the editor to create the task.\n")
	sb.WriteString("# Valid Priorities: L, M, H, C\n")
	sb.WriteString("#\n")
	sb.WriteString("Title: \n")
	sb.WriteString("Priority: M\n")
	if defaultProject != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", defaultProject))
	} else {
		sb.WriteString("Project: \n")
	}
	sb.WriteString("Blocker Reason: \n")
	sb.WriteString("\n")
	sb.WriteString("# Everything below 'Description:' will be treated as the task description.\n")
	sb.WriteString("Description:\n")
	return sb.String()
}

func FormatEditTaskTemplate(t *model.Task) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# TaskManager: Edit Task #%d\n", t.ID))
	sb.WriteString("# Lines starting with '#' are comments and will be ignored.\n")
	sb.WriteString("# Save and close the editor to commit your modifications.\n")
	sb.WriteString("# Valid Statuses: pending, in_progress, blocked, completed, cancelled\n")
	sb.WriteString("# Valid Priorities: L, M, H, C\n")
	sb.WriteString("#\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", t.Title))
	sb.WriteString(fmt.Sprintf("Status: %s\n", t.Status))
	sb.WriteString(fmt.Sprintf("Priority: %s\n", t.Priority))
	sb.WriteString(fmt.Sprintf("Project: %s\n", t.Project))
	sb.WriteString(fmt.Sprintf("Blocker Reason: %s\n", t.BlockerReason))
	sb.WriteString("\n")
	sb.WriteString("# Everything below 'Description:' will be treated as the task description.\n")
	sb.WriteString("Description:\n")
	if t.Description != "" {
		sb.WriteString(t.Description)
		if !strings.HasSuffix(t.Description, "\n") {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func ParseTaskTemplate(content string) (ParsedTask, error) {
	var res ParsedTask
	res.Priority = model.PriorityMedium
	res.Status = model.StatusPending

	scanner := bufio.NewScanner(strings.NewReader(content))
	inDescription := false
	var descLines []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Comments outside description are ignored
		if !inDescription && strings.HasPrefix(trimmed, "#") {
			continue
		}

		if inDescription {
			// Inside description section, lines starting with # are still comments if on top
			if strings.HasPrefix(trimmed, "#") && len(descLines) == 0 {
				continue
			}
			descLines = append(descLines, line)
			continue
		}

		if trimmed == "" {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(line[:colonIdx]))
		val := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "title":
			res.Title = val
		case "status":
			valLower := strings.ToLower(val)
			if valLower == "active" {
				valLower = "in_progress"
			}
			if valLower != "" {
				res.Status = model.TaskStatus(valLower)
			}
		case "priority":
			prio := model.TaskPriority(strings.ToUpper(val))
			switch prio {
			case model.PriorityLow, model.PriorityMedium, model.PriorityHigh, model.PriorityCritical:
				res.Priority = prio
			}
		case "project":
			res.Project = val
		case "blocker reason", "blocker", "block_reason":
			res.BlockerReason = val
		case "description":
			inDescription = true
			if val != "" {
				descLines = append(descLines, val)
			}
		}
	}

	res.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
	return res, nil
}

func ResolveEditorCommand(filePath string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if _, err := exec.LookPath("vim"); err == nil {
			editor = "vim"
		} else if _, err := exec.LookPath("nano"); err == nil {
			editor = "nano"
		} else {
			editor = "vi"
		}
	}

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return exec.Command("vi", filePath)
	}

	args := append(parts[1:], filePath)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}
