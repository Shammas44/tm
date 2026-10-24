package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
)

var (
	addPriority string
	addProject  string
	addDesc     string
)

var addCmd = &cobra.Command{
	Use:     "add <title> [priority:H|M|L|C] [project:<name>] [desc:<description>]",
	Aliases: []string{"new"},
	Short:   "Create a new task",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		var titleParts []string
		priority := addPriority
		proj := addProject
		if proj == "" {
			proj = projectFlag
		}
		desc := addDesc

		for _, arg := range args {
			argLower := strings.ToLower(arg)
			if strings.HasPrefix(argLower, "priority:") || strings.HasPrefix(argLower, "p:") {
				parts := strings.SplitN(arg, ":", 2)
				priority = strings.ToUpper(parts[1])
			} else if strings.HasPrefix(argLower, "project:") || strings.HasPrefix(argLower, "proj:") {
				parts := strings.SplitN(arg, ":", 2)
				proj = parts[1]
			} else if strings.HasPrefix(argLower, "desc:") || strings.HasPrefix(argLower, "description:") {
				parts := strings.SplitN(arg, ":", 2)
				desc = parts[1]
			} else {
				titleParts = append(titleParts, arg)
			}
		}

		title := strings.Join(titleParts, " ")
		if title == "" {
			return fmt.Errorf("task title cannot be empty")
		}

		taskPrio := model.TaskPriority(priority)
		if taskPrio == "" {
			taskPrio = model.PriorityMedium
		}

		task := &model.Task{
			Title:       title,
			Description: desc,
			Status:      model.StatusPending,
			Priority:    taskPrio,
			Project:     proj,
		}

		created, err := appStore.CreateTask(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		CheckpointActiveSession(ctx, &created.ID, fmt.Sprintf("Created task #%d: %s", created.ID, created.Title))

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(created)
		}

		if !quietFlag {
			fmt.Fprintf(cmd.OutOrStdout(), "Created task #%d: %s [priority:%s project:%s]\n",
				created.ID, created.Title, created.Priority, created.Project)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), created.ID)
		}
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addPriority, "priority", "p", "", "Task priority (L, M, H, C)")
	addCmd.Flags().StringVar(&addProject, "project", "", "Project name")
	addCmd.Flags().StringVarP(&addDesc, "desc", "d", "", "Task description")
	RootCmd.AddCommand(addCmd)
}
