package tui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"taskmanager/internal/model"
)

type ParsedTask struct {
	Title         string
	Status        model.TaskStatus
	Priority      model.TaskPriority
	Project       string
	BlockerReason string
	Tags          []string
	Dependencies  []int64
	Description   string
	NewNote       string
	ExistingNotes map[int64]string // parsed existing notes (id -> content)
}

func FormatNewTaskTemplate(defaultProject string) string {
	var sb strings.Builder
	sb.WriteString("# ==============================================================================\n")
	sb.WriteString("# TaskManager: Create New Task (vit-style editor)\n")
	sb.WriteString("# Lines starting with '#' are comments and will be ignored.\n")
	sb.WriteString("# Save and close the editor to create the task.\n")
	sb.WriteString("# Valid Priorities: L, M, H, C\n")
	sb.WriteString("# ==============================================================================\n\n")
	sb.WriteString("Title: \n")
	sb.WriteString("Priority: M\n")
	if defaultProject != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", defaultProject))
	} else {
		sb.WriteString("Project: \n")
	}
	sb.WriteString("Tags: \n")
	sb.WriteString("Depends: \n")
	sb.WriteString("Blocker Reason: \n")
	sb.WriteString("\n")
	sb.WriteString("# Optional: Enter an initial observation or technical note below:\n")
	sb.WriteString("New Note: \n")
	sb.WriteString("\n")
	sb.WriteString("# Everything below 'Description:' will be treated as the task description.\n")
	sb.WriteString("Description:\n")
	return sb.String()
}

func FormatEditTaskTemplate(t *model.Task, allTasks []*model.Task, observations []*model.Observation) string {
	var sb strings.Builder
	sb.WriteString("# ==============================================================================\n")
	sb.WriteString(fmt.Sprintf("# READ-ONLY TASK METADATA: Task #%d\n", t.ID))
	sb.WriteString(fmt.Sprintf("# Created: %s | Updated: %s\n",
		t.CreatedAt.Local().Format("2006-01-02 15:04:05"),
		t.UpdatedAt.Local().Format("2006-01-02 15:04:05"),
	))

	// Virtual tags
	vtags := model.ComputeVirtualTags(t, allTasks, len(observations) > 0)
	sb.WriteString(fmt.Sprintf("# Virtual Tags: %s\n", strings.Join(vtags, ", ")))

	// Dependencies status
	if len(t.Dependencies) > 0 {
		var depStrs []string
		for _, depID := range t.Dependencies {
			var otherStatus model.TaskStatus = "unknown"
			for _, other := range allTasks {
				if other.ID == depID {
					otherStatus = other.Status
					break
				}
			}
			depStrs = append(depStrs, fmt.Sprintf("#%d (%s)", depID, otherStatus))
		}
		sb.WriteString(fmt.Sprintf("# Dependencies: %s\n", strings.Join(depStrs, ", ")))
	}
	sb.WriteString("# ==============================================================================\n")
	sb.WriteString("# EDITABLE TASK FIELDS (Save and close editor to commit changes)\n")
	sb.WriteString("# Valid Statuses: pending, in_progress, blocked, completed, cancelled\n")
	sb.WriteString("# Valid Priorities: L, M, H, C\n\n")

	sb.WriteString(fmt.Sprintf("Title: %s\n", t.Title))
	sb.WriteString(fmt.Sprintf("Status: %s\n", t.Status))
	sb.WriteString(fmt.Sprintf("Priority: %s\n", t.Priority))
	sb.WriteString(fmt.Sprintf("Project: %s\n", t.Project))
	sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(t.Tags, ", ")))

	var depList []string
	for _, d := range t.Dependencies {
		depList = append(depList, strconv.FormatInt(d, 10))
	}
	sb.WriteString(fmt.Sprintf("Depends: %s\n", strings.Join(depList, ", ")))
	sb.WriteString(fmt.Sprintf("Blocker Reason: %s\n", t.BlockerReason))
	sb.WriteString("\n")

	sb.WriteString("# ==============================================================================\n")
	sb.WriteString("# OBSERVATIONS & NOTES (Edit note text to update, or delete line to remove)\n")
	sb.WriteString(fmt.Sprintf("# Total recorded: %d\n", len(observations)))
	sb.WriteString("# ==============================================================================\n")
	for _, o := range observations {
		contentTrimmed := strings.TrimSpace(o.Content)
		lines := strings.Split(contentTrimmed, "\n")
		if len(lines) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("Note %d: %s\n", o.ID, lines[0]))
		for _, subLine := range lines[1:] {
			sb.WriteString(fmt.Sprintf("  %s\n", subLine))
		}
	}
	sb.WriteString("\n")
	sb.WriteString("# Add a new observation/note to task history upon saving (multi-line supported):\n")
	sb.WriteString("New Note: \n")
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
	res.Tags = []string{}
	res.Dependencies = []int64{}
	res.ExistingNotes = make(map[int64]string)

	scanner := bufio.NewScanner(strings.NewReader(content))

	type parseState int
	const (
		stateHeader parseState = iota
		stateInNote
		stateInNewNote
		stateInDescription
	)

	curState := stateHeader
	currentNoteID := int64(0)
	var newNoteLines []string
	var descLines []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if curState == stateInDescription {
			if strings.HasPrefix(trimmed, "#") && len(descLines) == 0 {
				continue
			}
			descLines = append(descLines, line)
			continue
		}

		// Comment lines outside description
		if strings.HasPrefix(trimmed, "#") {
			if curState == stateInNewNote || curState == stateInNote {
				curState = stateHeader
				currentNoteID = 0
			}
			continue
		}

		if trimmed == "" {
			if curState == stateInNote {
				curState = stateHeader
				currentNoteID = 0
			} else if curState == stateInNewNote {
				newNoteLines = append(newNoteLines, "")
			}
			continue
		}

		// Check if line enters Description:
		if strings.EqualFold(trimmed, "description:") || strings.HasPrefix(strings.ToLower(trimmed), "description:") {
			curState = stateInDescription
			currentNoteID = 0
			afterColon := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			if afterColon != "" {
				descLines = append(descLines, afterColon)
			}
			continue
		}

		// Check if line starts New Note: or New Observation:
		if strings.HasPrefix(strings.ToLower(trimmed), "new note:") || strings.HasPrefix(strings.ToLower(trimmed), "new observation:") {
			curState = stateInNewNote
			currentNoteID = 0
			colonIdx := strings.Index(line, ":")
			afterColon := strings.TrimSpace(line[colonIdx+1:])
			if afterColon != "" {
				newNoteLines = append(newNoteLines, afterColon)
			}
			continue
		}

		// Check if line starts Note <id>: or Observation <id>:
		lower := strings.ToLower(trimmed)
		if (strings.HasPrefix(lower, "note ") || strings.HasPrefix(lower, "observation ")) && strings.Contains(line, ":") {
			colonIdx := strings.Index(line, ":")
			key := strings.TrimSpace(line[:colonIdx])
			val := strings.TrimSpace(line[colonIdx+1:])
			fields := strings.Fields(key)
			if len(fields) >= 2 {
				if id, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					curState = stateInNote
					currentNoteID = id
					res.ExistingNotes[id] = val
					continue
				}
			}
		}

		// Check if continuation line of current existing note
		if curState == stateInNote && currentNoteID > 0 {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
				if res.ExistingNotes[currentNoteID] == "" {
					res.ExistingNotes[currentNoteID] = trimmed
				} else {
					res.ExistingNotes[currentNoteID] += "\n" + trimmed
				}
				continue
			} else {
				curState = stateHeader
				currentNoteID = 0
			}
		}

		// Check if continuation line of New Note:
		if curState == stateInNewNote {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
				newNoteLines = append(newNoteLines, trimmed)
				continue
			} else {
				curState = stateHeader
			}
		}

		// Key-Value field parsing
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
		case "tags", "tag":
			if val != "" {
				rawTags := strings.FieldsFunc(val, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\t'
				})
				for _, t := range rawTags {
					tagClean := strings.TrimSpace(strings.TrimPrefix(t, "+"))
					if tagClean != "" {
						res.Tags = append(res.Tags, tagClean)
					}
				}
			}
		case "depends", "dependencies", "dep":
			if val != "" {
				rawDeps := strings.FieldsFunc(val, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\t'
				})
				for _, d := range rawDeps {
					dClean := strings.TrimSpace(strings.TrimPrefix(d, "#"))
					if id, err := strconv.ParseInt(dClean, 10, 64); err == nil {
						res.Dependencies = append(res.Dependencies, id)
					}
				}
			}
		case "blocker reason", "blocker", "block_reason":
			res.BlockerReason = val
		}
	}

	res.NewNote = strings.TrimSpace(strings.Join(newNoteLines, "\n"))
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
