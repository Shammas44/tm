package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"taskmanager/internal/output"
)

var tagCmd = &cobra.Command{
	Use:                "tag <id> [+tag|-tag...]",
	Aliases:            []string{"tags"},
	Short:              "Add or remove tags on an existing task",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, rawArgs []string) error {
		ctx := context.Background()

		// Filter out global flags if passed
		var args []string
		for i := 0; i < len(rawArgs); i++ {
			a := rawArgs[i]
			if (a == "--db" || a == "-P" || a == "--project") && i+1 < len(rawArgs) {
				i++
				continue
			}
			if strings.HasPrefix(a, "--db=") || strings.HasPrefix(a, "-P=") || strings.HasPrefix(a, "--project=") {
				continue
			}
			if a == "--json" {
				jsonFlag = true
				formatter = output.New(true, cmd.OutOrStdout())
				continue
			}
			if a == "-q" || a == "--quiet" {
				quietFlag = true
				continue
			}
			args = append(args, a)
		}

		if len(args) == 0 {
			return fmt.Errorf("task ID is required: tm tag <id> [+tag|-tag...]")
		}

		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID '%s': must be an integer", args[0])
		}

		task, err := appStore.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get task #%d: %w", id, err)
		}

		if len(args) == 1 {
			// Just list tags
			if formatter.ShouldUseJSON() {
				return formatter.PrintJSON(map[string]any{
					"id":   task.ID,
					"tags": task.Tags,
				})
			}
			if len(task.Tags) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Task #%d has no tags.\n", task.ID)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Task #%d tags: %s\n", task.ID, strings.Join(task.Tags, ", "))
			}
			return nil
		}

		currentTagMap := make(map[string]bool)
		for _, t := range task.Tags {
			currentTagMap[t] = true
		}

		for _, arg := range args[1:] {
			for _, part := range strings.Split(arg, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if strings.HasPrefix(part, "-") {
					delete(currentTagMap, strings.TrimPrefix(part, "-"))
				} else {
					tag := strings.TrimPrefix(part, "+")
					currentTagMap[tag] = true
				}
			}
		}

		var newTags []string
		for t := range currentTagMap {
			newTags = append(newTags, t)
		}
		task.Tags = newTags

		if err := appStore.UpdateTask(ctx, task); err != nil {
			return fmt.Errorf("failed to update tags for task #%d: %w", task.ID, err)
		}

		CheckpointActiveSession(ctx, &task.ID, fmt.Sprintf("Updated tags for task #%d: %s", task.ID, strings.Join(task.Tags, ", ")))

		updated, _ := appStore.GetTask(ctx, id)
		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(updated)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated task #%d tags: %s\n", updated.ID, strings.Join(updated.Tags, ", "))
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(tagCmd)
}
