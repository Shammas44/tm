package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"taskmanager/internal/model"
)

var (
	handoffNext string
	handoffFile string
)

type HandoffResponse struct {
	SessionID  string    `json:"session_id"`
	Status     string    `json:"status"`
	Handoff    any       `json:"handoff"`
	Summary    string    `json:"summary,omitempty"`     // backward-compatible
	NextAction string    `json:"next_action,omitempty"` // backward-compatible
	EndedAt    time.Time `json:"ended_at"`
}

var handoffCmd = &cobra.Command{
	Use:   "handoff [note or JSON] [--file <path>]",
	Short: "Record a clean session handoff as plain text or structured JSON for the incoming agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		var rawContent string
		if handoffFile != "" {
			bytes, err := os.ReadFile(handoffFile)
			if err != nil {
				return fmt.Errorf("failed to read handoff file %s: %w", handoffFile, err)
			}
			rawContent = strings.TrimSpace(string(bytes))
		} else if len(args) > 0 {
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
			joined := strings.Join(summaryParts, " ")
			if nextAction != "" && !strings.HasPrefix(strings.TrimSpace(joined), "{") {
				m := map[string]string{
					"summary":     joined,
					"next_action": nextAction,
				}
				b, _ := json.Marshal(m)
				rawContent = string(b)
			} else {
				rawContent = strings.Join(args, " ")
			}
		}

		if rawContent == "" {
			rawContent = "Session concluded"
		}

		// Validate / parse handoff content
		var parsedHandoff any
		if (strings.HasPrefix(rawContent, "{") && strings.HasSuffix(rawContent, "}")) ||
			(strings.HasPrefix(rawContent, "[") && strings.HasSuffix(rawContent, "]")) {
			if err := json.Unmarshal([]byte(rawContent), &parsedHandoff); err == nil {
				// Valid JSON
			} else {
				parsedHandoff = rawContent
			}
		} else {
			parsedHandoff = rawContent
		}

		activeSession, err := appStore.GetActiveSession(ctx)
		if err != nil || activeSession == nil {
			activeSession, err = appStore.StartSession(ctx, "developer", nil)
			if err != nil {
				return fmt.Errorf("failed to open session: %w", err)
			}
		}

		if err := appStore.CloseSession(ctx, activeSession.ID, rawContent); err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}

		now := time.Now().UTC()
		resp := HandoffResponse{
			SessionID: activeSession.ID,
			Status:    string(model.SessionStatusCompleted),
			Handoff:   parsedHandoff,
			EndedAt:   now,
		}

		// Backward-compatible summary & nextAction in response
		if m, ok := parsedHandoff.(map[string]any); ok {
			if sm, has := m["summary"]; has {
				resp.Summary = fmt.Sprint(sm)
			}
			if na, has := m["next_action"]; has {
				resp.NextAction = fmt.Sprint(na)
			} else if na, has := m["next"]; has {
				resp.NextAction = fmt.Sprint(na)
			}
		} else if str, ok := parsedHandoff.(string); ok {
			resp.Summary = str
		}

		if formatter.ShouldUseJSON() {
			return formatter.PrintJSON(resp)
		}

		if !quietFlag {
			fmt.Fprintln(cmd.OutOrStdout(), "=== Session Handoff Recorded ===")
			fmt.Fprintf(cmd.OutOrStdout(), "Session ID: %s\n", resp.SessionID)
			if str, ok := parsedHandoff.(string); ok {
				fmt.Fprintf(cmd.OutOrStdout(), "Handoff:    %s\n", str)
			} else {
				prettyJSON, _ := json.MarshalIndent(parsedHandoff, "  ", "  ")
				fmt.Fprintf(cmd.OutOrStdout(), "Handoff (JSON):\n  %s\n", string(prettyJSON))
			}
		}
		return nil
	},
}

func init() {
	handoffCmd.Flags().StringVarP(&handoffNext, "next", "n", "", "Recommended next action for future sessions")
	handoffCmd.Flags().StringVarP(&handoffFile, "file", "f", "", "Read handoff content from file (supports JSON or text)")
	RootCmd.AddCommand(handoffCmd)
}
