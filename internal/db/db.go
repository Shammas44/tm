package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

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

	return db, nil
}
