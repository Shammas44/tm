package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

type sessionYAML struct {
	ID           string           `yaml:"id"`
	Status       string           `yaml:"status"`
	Agent        string           `yaml:"agent"`
	StartedAt    string           `yaml:"started_at"`
	EndedAt      string           `yaml:"ended_at,omitempty"`
	Duration     string           `yaml:"duration"`
	ActiveTask   *sessionTaskYAML `yaml:"active_task,omitempty"`
	Checkpoint   string           `yaml:"checkpoint,omitempty"`
	Handoff      any              `yaml:"handoff,omitempty"`
	TasksTouched []sessionTaskRef `yaml:"tasks_touched,omitempty"`
	Observations []sessionObsYAML `yaml:"observations,omitempty"`
}

type sessionTaskYAML struct {
	ID       int64  `yaml:"id"`
	Title    string `yaml:"title"`
	Status   string `yaml:"status"`
	Priority string `yaml:"priority"`
	Project  string `yaml:"project,omitempty"`
}

type sessionTaskRef struct {
	ID       int64  `yaml:"id"`
	Title    string `yaml:"title"`
	Status   string `yaml:"status"`
	Priority string `yaml:"priority"`
}

type sessionObsYAML struct {
	ID        int64  `yaml:"id"`
	TaskID    *int64 `yaml:"task_id,omitempty"`
	Timestamp string `yaml:"timestamp"`
	Content   string `yaml:"content"`
}

// FormatSessionYAML serializes a complete session with its active task, touched tasks, and observations into clean YAML.
func FormatSessionYAML(sess *model.Session, activeTask *model.Task, obs []*model.Observation, touchedTasks []*model.Task) string {
	var sb strings.Builder
	sb.WriteString("# ==============================================================================\n")
	sb.WriteString(fmt.Sprintf("# READ-ONLY SESSION INSPECTION: %s\n", sess.ID))
	sb.WriteString("# This file is strictly read-only. Close your editor (:q) to return.\n")
	sb.WriteString("# ==============================================================================\n\n")

	durStr := FormatSessionDuration(sess)
	startedStr := sess.StartedAt.Local().Format("2006-01-02 15:04:05")
	endedStr := ""
	if sess.EndedAt != nil {
		endedStr = sess.EndedAt.Local().Format("2006-01-02 15:04:05")
	}

	data := sessionYAML{
		ID:         sess.ID,
		Status:     string(sess.Status),
		Agent:      sess.AgentID,
		StartedAt:  startedStr,
		EndedAt:    endedStr,
		Duration:   durStr,
		Checkpoint: sess.Checkpoint,
		Handoff:    sess.Handoff,
	}
	if data.Checkpoint == "" && sess.Summary != "" {
		data.Checkpoint = sess.Summary
	}

	if activeTask != nil {
		data.ActiveTask = &sessionTaskYAML{
			ID:       activeTask.ID,
			Title:    activeTask.Title,
			Status:   string(activeTask.Status),
			Priority: string(activeTask.Priority),
			Project:  activeTask.Project,
		}
	}

	for _, t := range touchedTasks {
		data.TasksTouched = append(data.TasksTouched, sessionTaskRef{
			ID:       t.ID,
			Title:    t.Title,
			Status:   string(t.Status),
			Priority: string(t.Priority),
		})
	}

	for _, o := range obs {
		data.Observations = append(data.Observations, sessionObsYAML{
			ID:        o.ID,
			TaskID:    o.TaskID,
			Timestamp: o.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			Content:   o.Content,
		})
	}

	yamlBytes, err := yaml.Marshal(&data)
	if err == nil {
		sb.Write(yamlBytes)
	}

	return sb.String()
}

func FormatSessionDuration(s *model.Session) string {
	var dur time.Duration
	if s.EndedAt != nil {
		dur = s.EndedAt.Sub(s.StartedAt)
	} else {
		dur = time.Since(s.StartedAt)
	}

	if dur < time.Second {
		return "<1s"
	}
	dur = dur.Round(time.Second)
	sec := int(dur.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	remSec := sec % 60
	if min < 60 {
		return fmt.Sprintf("%dm %ds", min, remSec)
	}
	hr := min / 60
	remMin := min % 60
	return fmt.Sprintf("%dh %dm", hr, remMin)
}

func (m Model) openSessionEditor(sessionID string) (Model, tea.Cmd) {
	ctx := context.Background()
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to load session %s: %v", sessionID, err)
		return m, nil
	}

	var activeTask *model.Task
	if sess.ActiveTaskID != nil {
		activeTask, _ = m.store.GetTask(ctx, *sess.ActiveTaskID)
	}

	obsList, _ := m.store.ListObservations(ctx, store.ObservationFilter{SessionID: sess.ID})

	touchedIDs := make(map[int64]bool)
	var touchedTasks []*model.Task
	if activeTask != nil {
		touchedIDs[activeTask.ID] = true
		touchedTasks = append(touchedTasks, activeTask)
	}
	for _, o := range obsList {
		if o.TaskID != nil && !touchedIDs[*o.TaskID] {
			touchedIDs[*o.TaskID] = true
			if t, err := m.store.GetTask(ctx, *o.TaskID); err == nil {
				touchedTasks = append(touchedTasks, t)
			}
		}
	}

	content := FormatSessionYAML(sess, activeTask, obsList, touchedTasks)

	shortID := sess.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	tempFile, err := os.CreateTemp("", fmt.Sprintf("tm-session-%s-*.yaml", shortID))
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to create temp file: %v", err)
		return m, nil
	}
	tempPath := tempFile.Name()
	_, _ = tempFile.WriteString(content)
	_ = tempFile.Close()

	cmd := ResolveEditorCommand(tempPath)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{
			mode:      "session",
			sessionID: sess.ID,
			tempPath:  tempPath,
			err:       err,
		}
	})
}
