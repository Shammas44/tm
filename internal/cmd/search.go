package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search across task notes and observations with FTS5 BM25 ranking",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		query := strings.Join(args, " ")

		results, err := appStore.SearchObservations(ctx, query)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		return formatter.PrintObservations(results)
	},
}

func init() {
	RootCmd.AddCommand(searchCmd)
}
