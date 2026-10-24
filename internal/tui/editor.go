package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"taskmanager/internal/model"
)

type ParsedTask struct {
	Title         string
	Status        model.TaskStatus
	Priority      model.TaskPriority
	Project       string
	Blocker       string
	Tags          []string
	Dependencies  []int64
	Description   string
	ExistingNotes map[int64]string // parsed existing notes (id -> content)
	NewNotes      []string         // newly added notes
}

type newTaskYAMLData struct {
	Title       string   `yaml:"title"`
	Priority    string   `yaml:"priority"`
	Project     string   `yaml:"project"`
	Tags        []string `yaml:"tags"`
	Depends     []int64  `yaml:"depends"`
	Blocker     string   `yaml:"blocker"`
	Notes       []string `yaml:"notes"`
	Description string   `yaml:"description"`
}

type editTaskYAMLData struct {
	Title       string           `yaml:"title"`
	Status      string           `yaml:"status"`
	Priority    string           `yaml:"priority"`
	Project     string           `yaml:"project"`
	Tags        []string         `yaml:"tags"`
	Depends     []int64          `yaml:"depends"`
	Blocker     string           `yaml:"blocker"`
	Notes       map[int64]string `yaml:"notes,omitempty"`
	Description string           `yaml:"description"`
}

func FormatNewTaskTemplate(defaultProject string) string {
	data := newTaskYAMLData{
		Title:       "",
		Priority:    "M",
		Project:     defaultProject,
		Tags:        []string{},
		Depends:     []int64{},
		Blocker:     "",
		Notes:       []string{},
		Description: "",
	}

	yamlBytes, _ := yaml.Marshal(&data)

	var sb strings.Builder
	sb.WriteString("# ==============================================================================\n")
	sb.WriteString("# TaskManager: Create New Task (YAML Format)\n")
	sb.WriteString("# Lines starting with '#' are comments and will be ignored.\n")
	sb.WriteString("# Valid Priorities: L, M, H, C\n")
	sb.WriteString("# Multi-line fields (blocker, description, notes): use '|-' format.\n")
	sb.WriteString("# Notes: add observations freely as list items or key-value entries under 'notes:'\n")
	sb.WriteString("# Save and close the editor to create the task.\n")
	sb.WriteString("# ==============================================================================\n\n")
	sb.Write(yamlBytes)
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
	sb.WriteString("# EDITABLE TASK DEFINITION (Save and close editor to commit changes)\n")
	sb.WriteString("# Valid Statuses: pending, started, blocked, completed, cancelled\n")
	sb.WriteString("# Valid Priorities: L, M, H, C\n")
	sb.WriteString("# Notes: modify existing IDs, delete lines to remove, or add new keys/notes freely\n")
	sb.WriteString("# Multi-line fields (blocker, description, notes): use '|-' format\n")
	sb.WriteString("# ==============================================================================\n\n")

	notesMap := make(map[int64]string)
	for _, o := range observations {
		notesMap[o.ID] = o.Content
	}

	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	deps := t.Dependencies
	if deps == nil {
		deps = []int64{}
	}

	data := editTaskYAMLData{
		Title:       t.Title,
		Status:      string(t.Status),
		Priority:    string(t.Priority),
		Project:     t.Project,
		Tags:        tags,
		Depends:     deps,
		Blocker:     t.Blocker,
		Notes:       notesMap,
		Description: t.Description,
	}

	yamlBytes, _ := yaml.Marshal(&data)
	sb.Write(yamlBytes)
	return sb.String()
}

func ParseTaskTemplate(content string) (ParsedTask, error) {
	var res ParsedTask
	res.Priority = model.PriorityMedium
	res.Status = model.StatusPending
	res.Tags = []string{}
	res.Dependencies = []int64{}
	res.ExistingNotes = make(map[int64]string)

	type rawYAML struct {
		Title         string `yaml:"title"`
		Status        string `yaml:"status"`
		Priority      string `yaml:"priority"`
		Project       string `yaml:"project"`
		Tags          any    `yaml:"tags"`
		Depends       any    `yaml:"depends"`
		Blocker       string `yaml:"blocker"`
		BlockerReason string `yaml:"blocker_reason"`
		Notes         any    `yaml:"notes"`
		Description   string `yaml:"description"`
	}

	var raw rawYAML
	err := yaml.Unmarshal([]byte(content), &raw)
	if err == nil && (raw.Title != "" || raw.Status != "" || raw.Project != "" || raw.Notes != nil || raw.Description != "") {
		res.Title = strings.TrimSpace(raw.Title)
		if raw.Status != "" {
			valLower := strings.ToLower(strings.TrimSpace(raw.Status))
			if valLower == "active" || valLower == "in_progress" {
				valLower = "started"
			}
			res.Status = model.TaskStatus(valLower)
		}
		if raw.Priority != "" {
			prio := model.TaskPriority(strings.ToUpper(strings.TrimSpace(raw.Priority)))
			switch prio {
			case model.PriorityLow, model.PriorityMedium, model.PriorityHigh, model.PriorityCritical:
				res.Priority = prio
			}
		}
		res.Project = strings.TrimSpace(raw.Project)
		res.Blocker = strings.TrimSpace(raw.Blocker)
		if res.Blocker == "" && raw.BlockerReason != "" {
			res.Blocker = strings.TrimSpace(raw.BlockerReason)
		}
		if res.Blocker != "" {
			res.Status = model.StatusBlocked
		}
		res.Description = strings.TrimSpace(raw.Description)

		// Parse tags (support list of strings or comma/space separated string)
		if raw.Tags != nil {
			switch tv := raw.Tags.(type) {
			case []any:
				for _, itm := range tv {
					tStr := strings.TrimSpace(strings.TrimPrefix(fmt.Sprint(itm), "+"))
					if tStr != "" {
						res.Tags = append(res.Tags, tStr)
					}
				}
			case []string:
				for _, itm := range tv {
					tStr := strings.TrimSpace(strings.TrimPrefix(itm, "+"))
					if tStr != "" {
						res.Tags = append(res.Tags, tStr)
					}
				}
			case string:
				parts := strings.FieldsFunc(tv, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\t'
				})
				for _, itm := range parts {
					tStr := strings.TrimSpace(strings.TrimPrefix(itm, "+"))
					if tStr != "" {
						res.Tags = append(res.Tags, tStr)
					}
				}
			}
		}

		// Parse depends (support list of ints/strings or comma separated string)
		if raw.Depends != nil {
			switch dv := raw.Depends.(type) {
			case []any:
				for _, itm := range dv {
					clean := strings.TrimSpace(strings.TrimPrefix(fmt.Sprint(itm), "#"))
					if id, err := strconv.ParseInt(clean, 10, 64); err == nil {
						res.Dependencies = append(res.Dependencies, id)
					}
				}
			case []int:
				for _, itm := range dv {
					res.Dependencies = append(res.Dependencies, int64(itm))
				}
			case []int64:
				res.Dependencies = append(res.Dependencies, dv...)
			case string:
				parts := strings.FieldsFunc(dv, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\t'
				})
				for _, itm := range parts {
					clean := strings.TrimSpace(strings.TrimPrefix(itm, "#"))
					if id, err := strconv.ParseInt(clean, 10, 64); err == nil {
						res.Dependencies = append(res.Dependencies, id)
					}
				}
			}
		}

		// Parse notes (supports map of id -> content, map with custom keys, or slice of notes)
		if raw.Notes != nil {
			switch nv := raw.Notes.(type) {
			case map[string]any:
				for k, v := range nv {
					vStr := strings.TrimRight(fmt.Sprint(v), "\r\n")
					if strings.TrimSpace(vStr) == "" {
						continue
					}
					kClean := strings.TrimSpace(strings.TrimPrefix(k, "#"))
					if id, err := strconv.ParseInt(kClean, 10, 64); err == nil && id > 0 {
						res.ExistingNotes[id] = vStr
					} else {
						res.NewNotes = append(res.NewNotes, vStr)
					}
				}
			case map[any]any:
				for k, v := range nv {
					vStr := strings.TrimRight(fmt.Sprint(v), "\r\n")
					if strings.TrimSpace(vStr) == "" {
						continue
					}
					kClean := strings.TrimSpace(strings.TrimPrefix(fmt.Sprint(k), "#"))
					if id, err := strconv.ParseInt(kClean, 10, 64); err == nil && id > 0 {
						res.ExistingNotes[id] = vStr
					} else {
						res.NewNotes = append(res.NewNotes, vStr)
					}
				}
			case []any:
				for _, item := range nv {
					vStr := strings.TrimRight(fmt.Sprint(item), "\r\n")
					if strings.TrimSpace(vStr) != "" {
						res.NewNotes = append(res.NewNotes, vStr)
					}
				}
			case []string:
				for _, item := range nv {
					vStr := strings.TrimRight(item, "\r\n")
					if strings.TrimSpace(vStr) != "" {
						res.NewNotes = append(res.NewNotes, vStr)
					}
				}
			case string:
				vStr := strings.TrimSpace(nv)
				if vStr != "" {
					res.NewNotes = append(res.NewNotes, vStr)
				}
			}
		}

		return res, nil
	}

	// Fallback legacy line-by-line parser if input is not standard YAML
	return parseLegacyTaskTemplate(content)
}

func parseLegacyTaskTemplate(content string) (ParsedTask, error) {
	var res ParsedTask
	res.Priority = model.PriorityMedium
	res.Status = model.StatusPending
	res.Tags = []string{}
	res.Dependencies = []int64{}
	res.ExistingNotes = make(map[int64]string)

	lines := strings.Split(content, "\n")
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

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if curState == stateInDescription {
			if strings.HasPrefix(trimmed, "#") && len(descLines) == 0 {
				continue
			}
			descLines = append(descLines, line)
			continue
		}

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

		if strings.EqualFold(trimmed, "description:") || strings.HasPrefix(strings.ToLower(trimmed), "description:") {
			curState = stateInDescription
			currentNoteID = 0
			afterColon := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			if afterColon != "" {
				descLines = append(descLines, afterColon)
			}
			continue
		}

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

		if curState == stateInNewNote {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
				newNoteLines = append(newNoteLines, trimmed)
				continue
			} else {
				curState = stateHeader
			}
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
			if valLower == "active" || valLower == "in_progress" {
				valLower = "started"
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
		case "blocker", "blocker reason", "block_reason":
			res.Blocker = val
			if strings.TrimSpace(res.Blocker) != "" {
				res.Status = model.StatusBlocked
			}
		}
	}

	newNoteStr := strings.TrimSpace(strings.Join(newNoteLines, "\n"))
	if newNoteStr != "" {
		res.NewNotes = append(res.NewNotes, newNoteStr)
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
		for _, candidate := range []string{"vim", "nano", "vi"} {
			if path, err := exec.LookPath(candidate); err == nil {
				editor = path
				break
			}
		}
	}
	if editor == "" {
		editor = "vim"
	}

	parts := strings.Fields(editor)
	parts = append(parts, filePath)
	return exec.Command(parts[0], parts[1:]...)
}
