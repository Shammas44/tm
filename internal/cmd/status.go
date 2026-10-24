package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

type StatusResponse struct {
	ActiveSession      *model.Session      `json:"active_session,omitempty"`
	LastSession        *model.Session      `json:"last_session,omitempty"`
	ActiveTask         *model.Task         `json:"active_task,omitempty"`
	Blockers           []string            `json:"blockers"`
	RecentObservations []string            `json:"recent_observations"`
	Backlog            store.BacklogCounts `json:"backlog"`
}

func (r StatusResponse) LastHandoffString() (string, bool) {
	if r.LastSession == nil || r.LastSession.Handoff == nil {
		return "", false
	}
	str, ok := r.LastSession.Handoff.(string)
	return str, ok
}

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "state", "info"},
	Short:   "Query project state, active task, blockers, and backlog without starting a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		resp := StatusResponse{
			Blockers:           []string{},
			RecentObservations: []string{},
		}

		// 1. Session status (active or last closed session, purely read-only)
		activeSession, err := appStore.GetActiveSession(ctx)
		if err == nil && activeSession != nil {
			resp.ActiveSession = activeSession
		} else {
			lastSession, err := appStore.GetLastSession(ctx)
			if err == nil && lastSession != nil {
				resp.LastSession = lastSession
			}
		}

		// 2. Active task (or suggested top priority pending task)
		activeTask, _ := appStore.GetActiveTask(ctx, projectFlag)
		if activeTask != nil {
			resp.ActiveTask = activeTask
		} else {
			pendingTasks, _ := appStore.ListTasks(ctx, store.TaskFilter{
				Project: projectFlag,
				Status:  model.StatusPending,
			})
			if len(pendingTasks) > 0 {
				resp.ActiveTask = pendingTasks[0]
			}
		}

		// 3. Open blockers
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

		// 4. Recent observations
		recentObs, _ := appStore.ListObservations(ctx, store.ObservationFilter{Limit: 5})
		for _, o := range recentObs {
			resp.RecentObservations = append(resp.RecentObservations, o.Content)
		}

		// 5. Backlog counts
		counts, err := appStore.GetBacklogCounts(ctx, projectFlag)
		if err == nil {
			resp.Backlog = counts
		}

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(resp)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "=== TaskManager Project Status ===")
		if resp.ActiveSession != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Active Session: %s (agent: %s, started: %s)\n",
				resp.ActiveSession.ID, resp.ActiveSession.AgentID, resp.ActiveSession.StartedAt.Local().Format("2006-01-02 15:04:05"))
			chk := resp.ActiveSession.Checkpoint
			if chk == "" {
				chk = resp.ActiveSession.Summary
			}
			if chk != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Last Checkpoint: %s\n", chk)
			}
		} else if resp.LastSession != nil {
			statusStr := string(resp.LastSession.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Last Session: %s [%s] (agent: %s)\n",
				resp.LastSession.ID, statusStr, resp.LastSession.AgentID)
			if resp.LastSession.Handoff != nil {
				if str, ok := resp.LastHandoffString(); ok && str != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Last Handoff: %s\n", str)
				} else {
					pretty, _ := json.MarshalIndent(resp.LastSession.Handoff, "  ", "  ")
					fmt.Fprintf(cmd.OutOrStdout(), "  Last Handoff (JSON):\n  %s\n", string(pretty))
				}
			} else if resp.LastSession.Summary != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Last Handoff: %s\n", resp.LastSession.Summary)
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Session: None recorded")
		}

		if resp.ActiveTask != nil {
			if resp.ActiveTask.Active {
				fmt.Fprintf(cmd.OutOrStdout(), "\nActive Task: #%d - %s [%s] (started)\n",
					resp.ActiveTask.ID, resp.ActiveTask.Title, resp.ActiveTask.Priority)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "\nSuggested Task (Pending): #%d - %s [%s]\n",
					resp.ActiveTask.ID, resp.ActiveTask.Title, resp.ActiveTask.Priority)
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "\nActive Task: None")
		}

		if len(resp.Blockers) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nOpen Blockers:")
			for _, b := range resp.Blockers {
				fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", b)
			}
		}

		if len(resp.RecentObservations) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nRecent Observations:")
			for _, o := range resp.RecentObservations {
				fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", o)
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\nBacklog: %d pending, %d started, %d blocked, %d completed\n",
			resp.Backlog.Pending, resp.Backlog.Started, resp.Backlog.Blocked, resp.Backlog.Completed)

		return nil
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}
