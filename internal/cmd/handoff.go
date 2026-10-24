package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
)

var handoffNext string

type HandoffResponse struct {
	SessionID  string    `json:"session_id"`
	Status     string    `json:"status"`
	Summary    string    `json:"summary"`
	NextAction string    `json:"next_action,omitempty"`
	EndedAt    time.Time `json:"ended_at"`
}

var handoffCmd = &cobra.Command{
	Use:   "handoff <summary> [next:<action>]",
	Short: "Record a clean session handoff summary and recommended next action",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		var summaryParts []string
		nextAction := handoffNext

		for _, arg := range args {
			argLower := strings.ToLower(arg)
			if strings.HasPrefix(argLower, "next:") {
				parts := strings.SplitN(arg, ":", 2)
				nextAction = parts[1]
			} else {
				summaryParts = append(summaryParts, arg)
			}
		}

		summary := strings.Join(summaryParts, " ")
		if summary == "" {
			summary = "Session concluded"
		}

		activeSession, err := appStore.GetActiveSession(ctx)
		if err != nil || activeSession == nil {
			activeSession, err = appStore.StartSession(ctx, "developer", nil)
			if err != nil {
				return fmt.Errorf("failed to open session: %w", err)
			}
		}

		if err := appStore.CloseSession(ctx, activeSession.ID, summary, nextAction); err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}

		now := time.Now().UTC()
		resp := HandoffResponse{
			SessionID:  activeSession.ID,
			Status:     string(model.SessionStatusCompleted),
			Summary:    summary,
			NextAction: nextAction,
			EndedAt:    now,
		}

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(resp)
		}

		if !quietFlag {
			fmt.Fprintln(cmd.OutOrStdout(), "=== Session Handoff Recorded ===")
			fmt.Fprintf(cmd.OutOrStdout(), "Session ID:  %s\n", resp.SessionID)
			fmt.Fprintf(cmd.OutOrStdout(), "Summary:     %s\n", resp.Summary)
			if resp.NextAction != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Next Action: %s\n", resp.NextAction)
			}
		}
		return nil
	},
}

func init() {
	handoffCmd.Flags().StringVarP(&handoffNext, "next", "n", "", "Recommended next action for future sessions")
	RootCmd.AddCommand(handoffCmd)
}
