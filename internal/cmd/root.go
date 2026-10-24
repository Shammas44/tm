package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"taskmanager/internal/db"
	"taskmanager/internal/output"
	"taskmanager/internal/store"
)

var (
	dbPath      string
	jsonFlag    bool
	quietFlag   bool
	projectFlag string

	sqlDB     *sql.DB
	appStore  *store.Store
	formatter *output.Formatter
)

var RootCmd = &cobra.Command{
	Use:   "tm",
	Short: "TaskManager: Autonomous AI-agent and developer task tracking engine",
	Long: `TaskManager ('tm') is a zero-dependency, crash-resilient CLI task manager
backed by an embedded pure-Go SQLite storage engine with FTS5 full-text search.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		resolvedDB := resolveDBPath(dbPath)

		// Open SQLite database with WAL and busy timeout
		database, err := db.Open(resolvedDB)
		if err != nil {
			return fmt.Errorf("failed to open database at %s: %w", resolvedDB, err)
		}
		sqlDB = database
		appStore = store.New(database)

		formatter = output.New(jsonFlag, cmd.OutOrStdout())
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	},
}

func Execute() error {
	rawArgs := os.Args[1:]
	processedArgs := PreprocessArgs(rawArgs)
	RootCmd.SetArgs(processedArgs)
	return RootCmd.Execute()
}

func init() {
	RootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to SQLite database file (default: .taskmanager/state.db or state.db)")
	RootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Force deterministic compact JSON output on stdout")
	RootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress informational messages")
	RootCmd.PersistentFlags().StringVarP(&projectFlag, "project", "P", "", "Project context filter")
}

func resolveDBPath(customPath string) string {
	if customPath != "" {
		return customPath
	}
	// Fallback: check os.Args in case a subcommand disabled flag parsing (e.g. tag, dep)
	for i, arg := range os.Args {
		if arg == "--db" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "--db=") {
			return strings.TrimPrefix(arg, "--db=")
		}
	}
	if env := os.Getenv("TASKMANAGER_DB"); env != "" {
		return env
	}

	proj := projectFlag
	if proj == "" {
		for i, arg := range os.Args {
			if (arg == "-P" || arg == "--project") && i+1 < len(os.Args) {
				proj = os.Args[i+1]
				break
			}
			if strings.HasPrefix(arg, "--project=") {
				proj = strings.TrimPrefix(arg, "--project=")
				break
			}
		}
	}
	if proj == "" {
		proj = os.Getenv("TASKMANAGER_PROJECT")
	}
	if proj != "" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".taskmanager", "projects", proj, "state.db")
		}
	}

	// Check if state.db already exists in current directory
	if _, err := os.Stat("state.db"); err == nil {
		return "state.db"
	}

	// Default to .taskmanager/state.db
	return filepath.Join(".taskmanager", "state.db")
}

// PreprocessArgs translates positional shortcut syntax e.g.:
// tm 1 start -> tm start 1
// tm 1 done  -> tm done 1
// tm 1 block <reason> -> tm block 1 <reason>
// tm 1 log <note> -> tm log 1 <note>
// tm 1 -> tm show 1
func PreprocessArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	var leadingFlags []string
	idx := 0

	// Consume flags before any positional argument
	for idx < len(args) {
		arg := args[idx]
		if strings.HasPrefix(arg, "-") {
			leadingFlags = append(leadingFlags, arg)
			if (arg == "--db" || arg == "-P" || arg == "--project" || arg == "--agent") && idx+1 < len(args) {
				idx++
				leadingFlags = append(leadingFlags, args[idx])
			}
			idx++
		} else {
			break
		}
	}

	if idx >= len(args) {
		return args
	}

	firstPos := args[idx]
	if _, err := strconv.ParseInt(firstPos, 10, 64); err == nil {
		// Exactly one positional: tm 1 -> tm show 1
		if idx+1 == len(args) {
			return append(leadingFlags, "show", firstPos)
		}

		action := args[idx+1]
		knownActions := map[string]bool{
			"start":  true,
			"done":   true,
			"block":  true,
			"log":    true,
			"show":   true,
			"edit":   true,
			"tag":    true,
			"tags":   true,
			"dep":    true,
			"deps":   true,
			"depend": true,
			"del":    true,
			"delete": true,
			"rm":     true,
		}

		if knownActions[action] {
			result := append([]string{}, leadingFlags...)
			result = append(result, action, firstPos)
			if idx+2 < len(args) {
				result = append(result, args[idx+2:]...)
			}
			return result
		}
	}

	return args
}

func CheckpointActiveSession(ctx context.Context, taskID *int64, note string) {
	if appStore == nil {
		return
	}
	activeSession, err := appStore.GetActiveSession(ctx)
	if err == nil && activeSession != nil {
		_ = appStore.UpdateCheckpoint(ctx, activeSession.ID, taskID, note)
	}
}
