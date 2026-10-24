package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

var (
	listStatus   string
	listPriority string
	listAll      bool
)

var listCmd = &cobra.Command{
	Use:     "list [filters...]",
	Aliases: []string{"ls"},
	Short:   "List tasks with optional status and priority filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		filter := store.TaskFilter{
			Project:        projectFlag,
			IncludeDeleted: listAll,
		}
		if listStatus != "" {
			filter.Status = model.TaskStatus(strings.ToLower(listStatus))
		}
		if listPriority != "" {
			filter.Priority = model.TaskPriority(strings.ToUpper(listPriority))
		}

		for _, arg := range args {
			argLower := strings.ToLower(arg)
			if argLower == "all" {
				filter.IncludeDeleted = true
				filter.Status = ""
			} else if argLower == "active" || argLower == "in_progress" {
				filter.Status = model.StatusInProgress
			} else if argLower == "pending" || argLower == "completed" || argLower == "blocked" || argLower == "cancelled" {
				filter.Status = model.TaskStatus(argLower)
			} else if strings.HasPrefix(argLower, "priority:") || strings.HasPrefix(argLower, "p:") {
				parts := strings.SplitN(arg, ":", 2)
				filter.Priority = model.TaskPriority(strings.ToUpper(parts[1]))
			} else if strings.HasPrefix(argLower, "project:") || strings.HasPrefix(argLower, "proj:") {
				parts := strings.SplitN(arg, ":", 2)
				filter.Project = parts[1]
			} else if strings.HasPrefix(argLower, "status:") || strings.HasPrefix(argLower, "s:") {
				parts := strings.SplitN(arg, ":", 2)
				val := strings.ToLower(parts[1])
				if val == "active" {
					val = "in_progress"
				}
				filter.Status = model.TaskStatus(val)
			}
		}

		tasks, err := appStore.ListTasks(ctx, filter)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// By default if no status specified and not --all, exclude completed and cancelled
		if filter.Status == "" && !listAll {
			var activeTasks []*model.Task
			for _, t := range tasks {
				if t.Status != model.StatusCompleted && t.Status != model.StatusCancelled {
					activeTasks = append(activeTasks, t)
				}
			}
			tasks = activeTasks
		}

		return formatter.PrintTasks(tasks)
	},
}

func init() {
	listCmd.Flags().StringVarP(&listStatus, "status", "s", "", "Filter by task status (pending, in_progress, blocked, completed)")
	listCmd.Flags().StringVarP(&listPriority, "priority", "p", "", "Filter by task priority (L, M, H, C)")
	listCmd.Flags().BoolVarP(&listAll, "all", "a", false, "Include completed and cancelled tasks")
	RootCmd.AddCommand(listCmd)
}
