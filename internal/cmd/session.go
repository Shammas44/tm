package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

var sessionLimit int

type SessionDetailResponse struct {
	Session      *model.Session       `json:"session"`
	Observations []*model.Observation `json:"observations"`
	TasksTouched []int64              `json:"tasks_touched"`
}

var sessionCmd = &cobra.Command{
	Use:     "session [list|show <id>]",
	Aliases: []string{"sessions"},
	Short:   "Inspect past sessions, audit interrupted runs, and view handoff history",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "list" {
			return runListSessions(cmd)
		}
		return runShowSession(cmd, args[0])
	},
}

var sessionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List historical sessions chronologically",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runListSessions(cmd)
	},
}

var sessionShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Inspect detailed notes, handoff, and observations of a specific session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runShowSession(cmd, args[0])
	},
}

func runListSessions(cmd *cobra.Command) error {
	ctx := context.Background()
	sessions, err := appStore.ListSessions(ctx, sessionLimit)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if formatter.ShouldUseJSON() {
		return formatter.PrintJSON(sessions)
	}

	if len(sessions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No sessions recorded.")
		return nil
	}

	colIDWidth := 10
	colStatusWidth := 13
	colAgentWidth := 18
	colStartedWidth := 17
	colDurWidth := 12
	colHandoffWidth := 30

	fmt.Fprintf(cmd.OutOrStdout(), "%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		colIDWidth, "Session",
		colStatusWidth, "Status",
		colAgentWidth, "Agent",
		colStartedWidth, "Started",
		colDurWidth, "Duration",
		"Handoff / Last Activity",
	)
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 100))

	for _, s := range sessions {
		shortID := s.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		durStr := formatSessionDuration(s)
		startedStr := s.StartedAt.Local().Format("2006-01-02 15:04")

		noteStr := ""
		if s.Handoff != nil {
			if str, ok := s.Handoff.(string); ok && str != "" {
				noteStr = str
			} else {
				b, _ := json.Marshal(s.Handoff)
				noteStr = string(b)
			}
		} else if s.Checkpoint != "" {
			noteStr = s.Checkpoint
		} else if s.Summary != "" {
			noteStr = s.Summary
		}

		if len(noteStr) > colHandoffWidth {
			noteStr = noteStr[:colHandoffWidth-3] + "..."
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
			colIDWidth, shortID,
			colStatusWidth, s.Status,
			colAgentWidth, truncateString(s.AgentID, colAgentWidth),
			colStartedWidth, startedStr,
			colDurWidth, durStr,
			noteStr,
		)
	}
	return nil
}

func runShowSession(cmd *cobra.Command, idOrPrefix string) error {
	ctx := context.Background()
	s, err := appStore.GetSession(ctx, idOrPrefix)
	if err != nil {
		return fmt.Errorf("session '%s' not found: %w", idOrPrefix, err)
	}

	obs, _ := appStore.ListObservations(ctx, store.ObservationFilter{
		SessionID: s.ID,
	})

	touchedMap := make(map[int64]bool)
	var touchedList []int64
	if s.ActiveTaskID != nil {
		touchedMap[*s.ActiveTaskID] = true
		touchedList = append(touchedList, *s.ActiveTaskID)
	}
	for _, o := range obs {
		if o.TaskID != nil && !touchedMap[*o.TaskID] {
			touchedMap[*o.TaskID] = true
			touchedList = append(touchedList, *o.TaskID)
		}
	}

	if formatter.ShouldUseJSON() {
		resp := SessionDetailResponse{
			Session:      s,
			Observations: obs,
			TasksTouched: touchedList,
		}
		return formatter.PrintJSON(resp)
	}

	durStr := formatSessionDuration(s)
	startedStr := s.StartedAt.Local().Format("2006-01-02 15:04:05")

	fmt.Fprintf(cmd.OutOrStdout(), "Session: %s\n", s.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s\n", s.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "  Agent:       %s\n", s.AgentID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Started:     %s (%s ago)\n", startedStr, formatTimeAgo(s.StartedAt))
	if s.EndedAt != nil {
		endedStr := s.EndedAt.Local().Format("2006-01-02 15:04:05")
		fmt.Fprintf(cmd.OutOrStdout(), "  Ended:       %s (duration: %s)\n", endedStr, durStr)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "  Duration:    %s\n", durStr)
	}

	if s.ActiveTaskID != nil {
		task, err := appStore.GetTask(ctx, *s.ActiveTaskID)
		if err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  Active Task: #%d - %s [%s] (%s)\n",
				task.ID, task.Title, task.Priority, task.Status)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  Active Task: #%d\n", *s.ActiveTaskID)
		}
	}

	if s.Checkpoint != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Checkpoint:  %s\n", s.Checkpoint)
	}

	if s.Handoff != nil {
		if str, ok := s.Handoff.(string); ok && str != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nHandoff:\n  %s\n", str)
		} else {
			prettyJSON, _ := json.MarshalIndent(s.Handoff, "  ", "  ")
			fmt.Fprintf(cmd.OutOrStdout(), "\nHandoff (JSON):\n  %s\n", string(prettyJSON))
		}
	}

	if len(obs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nObservations (%d recorded):\n", len(obs))
		for _, o := range obs {
			taskTag := ""
			if o.TaskID != nil {
				taskTag = fmt.Sprintf(" (Task #%d)", *o.TaskID)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  • [%s]%s %s\n",
				o.CreatedAt.Local().Format("2006-01-02 15:04:05"), taskTag, o.Content)
		}
	}

	return nil
}

func formatSessionDuration(s *model.Session) string {
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

func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return fmt.Sprintf("%ds", int(diff.Seconds()))
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd", int(diff.Hours()/24))
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		if maxLen > 3 {
			return s[:maxLen-3] + "..."
		}
		return s[:maxLen]
	}
	return s
}

func init() {
	sessionCmd.Flags().IntVar(&sessionLimit, "limit", 20, "Maximum number of sessions to list")
	sessionListCmd.Flags().IntVar(&sessionLimit, "limit", 20, "Maximum number of sessions to list")

	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	RootCmd.AddCommand(sessionCmd)
}
