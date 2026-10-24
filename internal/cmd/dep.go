package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"taskmanager/internal/output"
)

var depCmd = &cobra.Command{
	Use:                "dep <id> [+depID|-depID...]",
	Aliases:            []string{"deps", "depend"},
	Short:              "Add or remove task dependencies",
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
			return fmt.Errorf("task ID is required: tm dep <id> [+depID|-depID...]")
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
			// Just list dependencies
			if formatter.ShouldUseJSON() {
				return formatter.PrintJSON(map[string]any{
					"id":           task.ID,
					"dependencies": task.Dependencies,
				})
			}
			if len(task.Dependencies) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Task #%d has no dependencies.\n", task.ID)
			} else {
				var depStrs []string
				for _, d := range task.Dependencies {
					depStrs = append(depStrs, strconv.FormatInt(d, 10))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Task #%d depends on: %s\n", task.ID, strings.Join(depStrs, ", "))
			}
			return nil
		}

		depMap := make(map[int64]bool)
		for _, d := range task.Dependencies {
			depMap[d] = true
		}

		for _, arg := range args[1:] {
			for _, part := range strings.Split(arg, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if strings.HasPrefix(part, "-") {
					depID, err := strconv.ParseInt(strings.TrimPrefix(part, "-"), 10, 64)
					if err == nil {
						delete(depMap, depID)
					}
				} else {
					cleanPart := strings.TrimPrefix(part, "+")
					depID, err := strconv.ParseInt(cleanPart, 10, 64)
					if err == nil {
						if depID != task.ID {
							depMap[depID] = true
						}
					}
				}
			}
		}

		var newDeps []int64
		for d := range depMap {
			newDeps = append(newDeps, d)
		}
		task.Dependencies = newDeps

		if err := appStore.UpdateTask(ctx, task); err != nil {
			return fmt.Errorf("failed to update dependencies for task #%d: %w", task.ID, err)
		}

		var depStrs []string
		for _, d := range task.Dependencies {
			depStrs = append(depStrs, strconv.FormatInt(d, 10))
		}

		CheckpointActiveSession(ctx, &task.ID, fmt.Sprintf("Updated dependencies for task #%d: %s", task.ID, strings.Join(depStrs, ", ")))

		updated, _ := appStore.GetTask(ctx, id)
		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(updated)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated task #%d dependencies: %s\n", updated.ID, strings.Join(depStrs, ", "))
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(depCmd)
}
