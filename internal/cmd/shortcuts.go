package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
)

var startCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Mark a task as in_progress and set as the active task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		if err := appStore.SetActiveTask(ctx, id); err != nil {
			return fmt.Errorf("failed to start task #%d: %w", id, err)
		}

		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		CheckpointActiveSession(ctx, &task.ID, fmt.Sprintf("Started task #%d: %s", task.ID, task.Title))

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(task)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Started task #%d: %s\n", task.ID, task.Title)
		}
		return nil
	},
}

var doneCmd = &cobra.Command{
	Use:   "done <id>",
	Short: "Mark a task as completed and clear active status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		if err := appStore.UpdateTaskStatus(ctx, id, model.StatusCompleted, ""); err != nil {
			return fmt.Errorf("failed to complete task #%d: %w", id, err)
		}

		// Record a completion observation
		activeSession, _ := appStore.GetActiveSession(ctx)
		var sessionID *string
		if activeSession != nil {
			sessionID = &activeSession.ID
		}
		_, _ = appStore.AddObservation(ctx, sessionID, &task.ID, fmt.Sprintf("Completed task #%d: %s", task.ID, task.Title))

		CheckpointActiveSession(ctx, nil, fmt.Sprintf("Completed task #%d: %s", task.ID, task.Title))

		updated, _ := appStore.GetTask(ctx, id)

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(updated)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Completed task #%d: %s\n", task.ID, task.Title)
		}
		return nil
	},
}

var blockCmd = &cobra.Command{
	Use:   "block <id> <reason>",
	Short: "Mark a task as blocked with a reason",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		reason := strings.Join(args[1:], " ")
		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		if err := appStore.UpdateTaskStatus(ctx, id, model.StatusBlocked, reason); err != nil {
			return fmt.Errorf("failed to block task #%d: %w", id, err)
		}

		// Record blocker observation
		activeSession, _ := appStore.GetActiveSession(ctx)
		var sessionID *string
		if activeSession != nil {
			sessionID = &activeSession.ID
		}
		_, _ = appStore.AddObservation(ctx, sessionID, &task.ID, fmt.Sprintf("Blocked task #%d: %s", task.ID, reason))

		CheckpointActiveSession(ctx, nil, fmt.Sprintf("Blocked task #%d: %s", task.ID, reason))

		updated, _ := appStore.GetTask(ctx, id)

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(updated)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Blocked task #%d: %s\n", task.ID, reason)
		}
		return nil
	},
}

var logCmd = &cobra.Command{
	Use:   "log <id> <note>",
	Short: "Append an observation/note to a task and update session checkpoint",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		note := strings.Join(args[1:], " ")

		activeSession, _ := appStore.GetActiveSession(ctx)
		var sessionID *string
		if activeSession != nil {
			sessionID = &activeSession.ID
		}

		obs, err := appStore.AddObservation(ctx, sessionID, &task.ID, note)
		if err != nil {
			return fmt.Errorf("failed to save observation: %w", err)
		}

		CheckpointActiveSession(ctx, &task.ID, fmt.Sprintf("Task #%d note: %s", task.ID, note))

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(obs)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Logged note on task #%d: %s\n", task.ID, note)
		}
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show detailed view of a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		return formatter.PrintTask(task)
	},
}

var deleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"del", "rm"},
	Short:   "Cancel/delete a task",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		if err := appStore.DeleteTask(ctx, id); err != nil {
			return fmt.Errorf("failed to delete task #%d: %w", id, err)
		}

		CheckpointActiveSession(ctx, nil, fmt.Sprintf("Cancelled task #%d: %s", id, task.Title))

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(map[string]any{
				"cancelled": true,
				"id":        id,
				"title":     task.Title,
			})
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Cancelled task #%d: %s\n", id, task.Title)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(startCmd)
	RootCmd.AddCommand(doneCmd)
	RootCmd.AddCommand(blockCmd)
	RootCmd.AddCommand(logCmd)
	RootCmd.AddCommand(showCmd)
	RootCmd.AddCommand(deleteCmd)
}
