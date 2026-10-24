package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"taskmanager/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive full-screen terminal dashboard for task navigation and status toggling",
	RunE: func(cmd *cobra.Command, args []string) error {
		if appStore == nil {
			return fmt.Errorf("database store not initialized")
		}
		return tui.Run(appStore)
	},
}

func init() {
	RootCmd.AddCommand(tuiCmd)
}
