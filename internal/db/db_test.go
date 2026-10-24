package db

import (
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "state.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Verify tables exist
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count)
	if err != nil {
		t.Fatalf("Query tasks table failed: %v", err)
	}

	err = database.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	if err != nil {
		t.Fatalf("Query sessions table failed: %v", err)
	}

	err = database.QueryRow("SELECT COUNT(*) FROM observations").Scan(&count)
	if err != nil {
		t.Fatalf("Query observations table failed: %v", err)
	}

	err = database.QueryRow("SELECT COUNT(*) FROM observations_fts").Scan(&count)
	if err != nil {
		t.Fatalf("Query observations_fts table failed: %v", err)
	}
}
