package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
	"taskmanager/internal/tui"
)

var (
	editTitle       string
	editStatus      string
	editPriority    string
	editProject     string
	editDesc        string
	editTags        string
	editDeps        string
	editBlockReason string
)

var editCmd = &cobra.Command{
	Use:   "edit <id> [flags]",
	Short: "Edit a task via flags or interactively in $EDITOR",
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

		flagsChanged := cmd.Flags().Changed("title") ||
			cmd.Flags().Changed("status") ||
			cmd.Flags().Changed("priority") ||
			cmd.Flags().Changed("project") ||
			cmd.Flags().Changed("desc") ||
			cmd.Flags().Changed("tags") ||
			cmd.Flags().Changed("depends") ||
			cmd.Flags().Changed("block-reason")

		// 1. Non-interactive flag-based editing
		if flagsChanged {
			if cmd.Flags().Changed("title") {
				if strings.TrimSpace(editTitle) == "" {
					return fmt.Errorf("task title cannot be empty")
				}
				task.Title = editTitle
			}
			if cmd.Flags().Changed("status") {
				task.Status = model.TaskStatus(editStatus)
			}
			if cmd.Flags().Changed("priority") {
				task.Priority = model.TaskPriority(strings.ToUpper(editPriority))
			}
			if cmd.Flags().Changed("project") {
				task.Project = editProject
			}
			if cmd.Flags().Changed("desc") {
				task.Description = strings.ReplaceAll(editDesc, "\\n", "\n")
			}
			if cmd.Flags().Changed("block-reason") {
				task.BlockerReason = strings.ReplaceAll(editBlockReason, "\\n", "\n")
			}
			if cmd.Flags().Changed("tags") {
				var tags []string
				for _, part := range strings.Split(editTags, ",") {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						tags = append(tags, strings.TrimPrefix(trimmed, "+"))
					}
				}
				task.Tags = tags
			}
			if cmd.Flags().Changed("depends") {
				var deps []int64
				for _, part := range strings.Split(editDeps, ",") {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						if depID, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
							deps = append(deps, depID)
						}
					}
				}
				task.Dependencies = deps
			}

			if task.Status == model.StatusInProgress {
				_ = appStore.SetActiveTask(ctx, task.ID)
			}
			if err := appStore.UpdateTask(ctx, task); err != nil {
				return fmt.Errorf("failed to update task #%d: %w", task.ID, err)
			}

			CheckpointActiveSession(ctx, &task.ID, fmt.Sprintf("Updated task #%d: %s", task.ID, task.Title))

			updated, _ := appStore.GetTask(ctx, id)
			if formatter.ShouldUseJSON() {
				return formatter.PrintJSON(updated)
			}
			if !quietFlag {
				fmt.Fprintf(cmd.OutOrStdout(), "Updated task #%d: %s\n", updated.ID, updated.Title)
			}
			return nil
		}

		// 2. Interactive editor editing
		if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
			return fmt.Errorf("cannot open interactive editor in non-interactive shell; specify flags (e.g. --title, --status, --tags)")
		}

		allTasks, _ := appStore.ListTasks(ctx, store.TaskFilter{IncludeDeleted: false})
		obs, _ := appStore.ListObservations(ctx, store.ObservationFilter{TaskID: &task.ID})

		tmpFile, err := os.CreateTemp("", fmt.Sprintf("tm-cli-edit-%d-*.yaml", task.ID))
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		tmpl := tui.FormatEditTaskTemplate(task, allTasks, obs)
		if _, err := tmpFile.WriteString(tmpl); err != nil {
			_ = tmpFile.Close()
			return fmt.Errorf("failed to write template: %w", err)
		}
		_ = tmpFile.Close()

		execCmd := tui.ResolveEditorCommand(tmpFile.Name())
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			return fmt.Errorf("editor exited with error: %w", err)
		}

		contentBytes, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			return fmt.Errorf("failed to read editor content: %w", err)
		}

		parsed, err := tui.ParseTaskTemplate(string(contentBytes))
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}

		if strings.TrimSpace(parsed.Title) == "" {
			if !quietFlag {
				fmt.Fprintln(cmd.OutOrStdout(), "Edit cancelled (empty title).")
			}
			return nil
		}

		task.Title = parsed.Title
		task.Status = parsed.Status
		task.Priority = parsed.Priority
		task.Project = parsed.Project
		task.Description = parsed.Description
		task.BlockerReason = parsed.BlockerReason
		task.Tags = parsed.Tags
		task.Dependencies = parsed.Dependencies

		if task.Status == model.StatusInProgress {
			_ = appStore.SetActiveTask(ctx, task.ID)
		}
		if err := appStore.UpdateTask(ctx, task); err != nil {
			return fmt.Errorf("failed to update task #%d: %w", task.ID, err)
		}

		// Observations update/delete
		for _, o := range obs {
			if newContent, found := parsed.ExistingNotes[o.ID]; found {
				trimmedContent := strings.TrimSpace(newContent)
				if trimmedContent == "" {
					_ = appStore.DeleteObservation(ctx, o.ID)
				} else if trimmedContent != o.Content {
					_ = appStore.UpdateObservation(ctx, o.ID, trimmedContent)
				}
			} else {
				_ = appStore.DeleteObservation(ctx, o.ID)
			}
		}

		// New Note
		activeSession, _ := appStore.GetActiveSession(ctx)
		var sessionID *string
		if activeSession != nil {
			sessionID = &activeSession.ID
		}
		if strings.TrimSpace(parsed.NewNote) != "" {
			_, _ = appStore.AddObservation(ctx, sessionID, &task.ID, parsed.NewNote)
		}

		CheckpointActiveSession(ctx, &task.ID, fmt.Sprintf("Updated task #%d: %s", task.ID, task.Title))

		updated, _ := appStore.GetTask(ctx, id)
		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(updated)
		}
		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated task #%d: %s\n", updated.ID, updated.Title)
		}
		return nil
	},
}

func init() {
	editCmd.Flags().StringVar(&editTitle, "title", "", "Task title")
	editCmd.Flags().StringVar(&editStatus, "status", "", "Task status (pending, in_progress, blocked, completed, cancelled)")
	editCmd.Flags().StringVarP(&editPriority, "priority", "p", "", "Task priority (L, M, H, C)")
	editCmd.Flags().StringVarP(&editProject, "project", "P", "", "Project slug")
	editCmd.Flags().StringVar(&editDesc, "desc", "", "Task description")
	editCmd.Flags().StringVar(&editTags, "tags", "", "Comma-separated tags")
	editCmd.Flags().StringVar(&editDeps, "depends", "", "Comma-separated prerequisite task IDs")
	editCmd.Flags().StringVar(&editBlockReason, "block-reason", "", "Reason why task is blocked")

	RootCmd.AddCommand(editCmd)
}
