package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open initializes the SQLite database connection, applies PRAGMAs, and runs migrations.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
		}
	}

	connStr := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply schema migrations
	if _, err := db.Exec(SchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply database schema: %w", err)
	}

	// Incremental migrations for existing databases
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN tags TEXT DEFAULT '[]'")
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN dependencies TEXT DEFAULT '[]'")
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN blocker TEXT DEFAULT ''")
	_, _ = db.Exec("UPDATE tasks SET blocker = blocker_reason WHERE (blocker = '' OR blocker IS NULL) AND (blocker_reason != '' AND blocker_reason IS NOT NULL)")
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN checkpoint TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN handoff TEXT DEFAULT ''")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at DESC)")
	_, _ = db.Exec("UPDATE sessions SET handoff = summary WHERE (handoff = '' OR handoff IS NULL) AND (summary != '' AND summary IS NOT NULL)")

	// Migrate tasks table check constraint to support 'started' if needed
	var tasksSQL string
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&tasksSQL)
	if err == nil && !strings.Contains(tasksSQL, "'started'") {
		migration := `
			PRAGMA foreign_keys = OFF;
			CREATE TABLE IF NOT EXISTS tasks_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				title TEXT NOT NULL,
				description TEXT DEFAULT '',
				status TEXT NOT NULL CHECK(status IN ('pending', 'started', 'in_progress', 'completed', 'blocked', 'cancelled')),
				priority TEXT NOT NULL CHECK(priority IN ('L', 'M', 'H', 'C')),
				project TEXT DEFAULT '',
				active INTEGER DEFAULT 0,
				blocker TEXT DEFAULT '',
				tags TEXT DEFAULT '[]',
				dependencies TEXT DEFAULT '[]',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				completed_at TIMESTAMP
			);
			INSERT INTO tasks_new (id, title, description, status, priority, project, active, blocker, tags, dependencies, created_at, updated_at, completed_at)
			SELECT id, title, description, CASE WHEN status = 'in_progress' THEN 'started' ELSE status END, priority, project, active, blocker, tags, dependencies, created_at, updated_at, completed_at FROM tasks;
			DROP TABLE tasks;
			ALTER TABLE tasks_new RENAME TO tasks;
			CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
			CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project);
			CREATE INDEX IF NOT EXISTS idx_tasks_active ON tasks(active);
			PRAGMA foreign_keys = ON;
		`
		_, _ = db.Exec(migration)
	}
	_, _ = db.Exec("UPDATE tasks SET status = 'started' WHERE status = 'in_progress'")

	return db, nil
}
