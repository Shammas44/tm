package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

var initAgentID string

type InitResponse struct {
	SessionID            string              `json:"session_id"`
	ResumedFromInterrupt bool                `json:"resumed_from_interrupt"`
	LastHandoff          string              `json:"last_handoff,omitempty"`
	ActiveTask           *model.Task         `json:"active_task"`
	Blockers             []string            `json:"blockers"`
	RecentObservations   []string            `json:"recent_observations"`
	Backlog              store.BacklogCounts `json:"backlog"`
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize task context, detect interrupted sessions, and recover working memory",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		resp := InitResponse{
			Blockers:           []string{},
			RecentObservations: []string{},
		}

		// 1. Check for interrupted active session
		activeSession, err := appStore.GetActiveSession(ctx)
		if err == nil && activeSession != nil {
			resp.ResumedFromInterrupt = true
			if activeSession.Summary != "" {
				resp.LastHandoff = activeSession.Summary
			} else {
				resp.LastHandoff = fmt.Sprintf("Interrupted session %s", activeSession.ID[:8])
			}
			_ = appStore.MarkInterrupted(ctx, activeSession.ID)
		} else {
			// Check last closed session for handoff summary
			lastSession, err := appStore.GetLastSession(ctx)
			if err == nil && lastSession != nil && lastSession.Summary != "" {
				resp.LastHandoff = lastSession.Summary
			}
		}

		// 2. Determine active task (or top priority pending task)
		activeTask, _ := appStore.GetActiveTask(ctx, projectFlag)
		if activeTask != nil {
			resp.ActiveTask = activeTask
		} else {
			// Suggest top priority pending task if available
			pendingTasks, _ := appStore.ListTasks(ctx, store.TaskFilter{
				Project: projectFlag,
				Status:  model.StatusPending,
			})
			if len(pendingTasks) > 0 {
				resp.ActiveTask = pendingTasks[0]
			}
		}

		// 3. Find open blockers
		blockedTasks, _ := appStore.ListTasks(ctx, store.TaskFilter{
			Project: projectFlag,
			Status:  model.StatusBlocked,
		})
		for _, bt := range blockedTasks {
			reason := bt.Blocker
			if reason == "" {
				reason = "Blocked"
			}
			resp.Blockers = append(resp.Blockers, fmt.Sprintf("Task #%d (%s): %s", bt.ID, bt.Title, reason))
		}

		// 4. Fetch recent observations
		recentObs, _ := appStore.ListObservations(ctx, store.ObservationFilter{Limit: 5})
		for _, o := range recentObs {
			resp.RecentObservations = append(resp.RecentObservations, o.Content)
		}

		// 5. Backlog counts
		counts, err := appStore.GetBacklogCounts(ctx, projectFlag)
		if err == nil {
			resp.Backlog = counts
		}

		// 6. Start new session
		var activeTaskID *int64
		if resp.ActiveTask != nil && resp.ActiveTask.Active {
			activeTaskID = &resp.ActiveTask.ID
		}
		newSession, err := appStore.StartSession(ctx, initAgentID, activeTaskID)
		if err != nil {
			return fmt.Errorf("failed to start session: %w", err)
		}
		resp.SessionID = newSession.ID

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(resp)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "=== TaskManager Session Initialized ===")
		fmt.Fprintf(cmd.OutOrStdout(), "Session ID: %s\n", resp.SessionID)
		if resp.ResumedFromInterrupt {
			fmt.Fprintf(cmd.OutOrStdout(), "[RECOVERED FROM INTERRUPT] Last checkpoint: %s\n", resp.LastHandoff)
		} else if resp.LastHandoff != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "[LAST HANDOFF] %s\n", resp.LastHandoff)
		}

		if resp.ActiveTask != nil {
			statusStr := string(resp.ActiveTask.Status)
			if resp.ActiveTask.Active {
				statusStr = "active (in_progress)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nTarget Task: #%d - %s [%s] (%s)\n",
				resp.ActiveTask.ID, resp.ActiveTask.Title, resp.ActiveTask.Priority, statusStr)
		}

		if len(resp.Blockers) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nOpen Blockers:")
			for _, b := range resp.Blockers {
				fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", b)
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\nBacklog: %d pending, %d in progress, %d blocked, %d completed\n",
			resp.Backlog.Pending, resp.Backlog.InProgress, resp.Backlog.Blocked, resp.Backlog.Completed)

		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initAgentID, "agent", "developer", "Agent or developer identifier")
	RootCmd.AddCommand(initCmd)
}
