# Implementation Roadmap: TaskManager CLI (Go Rewrite)

This document outlines the sequential implementation plan for rewriting **TaskManager** as a single, zero-dependency Go CLI tool (`tm`). It replaces the previous Python MCP server, Taskwarrior CLI bridge, and Ed25519 JWT manager.

---

## Architecture Overview

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│ Developer Terminal (macOS) / Agent Shell (Docker / Sandcastle)              │
│                                                                             │
│  Commands:                                                                  │
│    • tm init [--json]                                                       │
│    • tm add "Task title" [priority:H|M|L|C] [project:<slug>]                │
│    • tm list / tm ls [status:...] [priority:...] [--json]                   │
│    • tm <id> start | done | block <reason> | log <note>                     │
│    • tm search <query> [--json]                                             │
│    • tm handoff [summary] [next:<action>] [--json]                          │
│    • tm tui (Interactive Bubbletea dashboard)                               │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │ Direct embedded SQLite calls
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Embedded Pure-Go SQLite Engine (`modernc.org/sqlite`)                       │
│                                                                             │
│  File: `<project_dir>/state.db`                                             │
│  Settings: PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;               │
│  Tables:                                                                    │
│    • tasks (auto-increment IDs: 1, 2, 3...)                                 │
│    • sessions (checkpoints, auto-recovery on interrupt)                     │
│    • observations (notes & findings)                                        │
│    • observations_fts (FTS5 full-text search)                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Go Project Skeleton & SQLite Storage Engine

### Slice 1.1: Project Setup & Dependencies
* **Objective:** Initialize the Go module and core dependencies.
* **Deliverables:**
  * `go.mod` (module `taskmanager`).
  * Pure Go dependencies:
    * `modernc.org/sqlite` (pure Go SQLite, zero CGO).
    * `github.com/spf13/cobra` (CLI routing and flags).
  * Directory layout:
    * `cmd/tm/main.go`
    * `internal/db/`
    * `internal/store/`
    * `internal/model/`
    * `internal/output/`
* **Validation Criteria:** `go build ./...` compiles cleanly without CGO enabled (`CGO_ENABLED=0`).

### Slice 1.2: Database Migration & Storage Engine
* **Objective:** Implement SQLite connection manager with WAL mode, busy timeout, and schema initialization.
* **Deliverables:**
  * `internal/db/db.go`: SQLite opener with `PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`.
  * `internal/db/schema.sql`:
    * `tasks` table with auto-increment integer IDs (`1`, `2`, `3`...).
    * `sessions` table tracking active/interrupted/completed state and checkpoint notes.
    * `observations` table storing timestamped findings and notes.
    * `observations_fts` virtual table using SQLite FTS5 with insert trigger.
* **Validation Criteria:** Unit tests verify migrations run idempotently on empty and existing database files.

### Slice 1.3: Data Access Layer (CRUD & FTS5)
* **Objective:** Implement domain models and store methods.
* **Deliverables:**
  * `internal/store/task_store.go`:
    * `CreateTask(title, priority, project, description)`
    * `GetTask(id)`
    * `ListTasks(filter)`
    * `UpdateTaskStatus(id, status, blockerReason)`
    * `SetActiveTask(id)`
  * `internal/store/observation_store.go`:
    * `AddObservation(sessionID, taskID, content)`
    * `SearchObservations(query)` (using FTS5 BM25 match)
  * `internal/store/session_store.go`:
    * `GetActiveSession()`
    * `StartSession(agentID, activeTaskID)`
    * `UpdateCheckpoint(sessionID, activeTaskID, note)`
    * `CloseSession(sessionID, summary, nextAction)`
    * `MarkInterrupted(sessionID)`
* **Validation Criteria:** Unit tests verifying task state transitions, FTS5 query results, and atomic checkpoint persistence.

---

## Phase 2: Core Taskwarrior-Style CLI Commands & Output Formatter

### Slice 2.1: Dual Output Formatter (Human TTY vs. Machine JSON)
* **Objective:** Provide human-readable ANSI tables when connected to a terminal, and compact deterministic JSON when piped or when `--json` is passed.
* **Deliverables:**
  * `internal/output/formatter.go`:
    * Auto-detection of terminal TTY (`mattn/go-isatty`).
    * Compact JSON encoder on `stdout`.
    * Clean table printer for tasks and search results.
* **Validation Criteria:** Commands print styled text in a terminal, and structured JSON with `--json` or redirected stdout.

### Slice 2.2: Task Creation, Listing & Filtering
* **Objective:** Implement `tm add` and `tm list` (alias `tm ls`) with Taskwarrior-style arguments.
* **Syntax:**
  * `tm add "Implement multiplication" priority:H project:c-calc`
  * `tm list [status:pending|in_progress|blocked|completed] [priority:H|M|L|C]`
  * `tm ls`
* **Validation Criteria:** Tasks can be added and listed with accurate filtering in both table and JSON output modes.

### Slice 2.3: Task State Mutations
* **Objective:** Fast positional commands to mutate task states.
* **Syntax:**
  * `tm <id> start`: sets task status to `in_progress` and marks it active.
  * `tm <id> done`: marks task `completed` and clears active status.
  * `tm <id> block <reason>`: marks task `blocked`, records reason, and unsets active.
  * `tm <id> log <note>`: appends observation to task and records in FTS5 index.
* **Validation Criteria:** State changes persist immediately to SQLite; invalid ID errors return exit code 1 with clear messages.

### Slice 2.4: Memory Full-Text Search
* **Objective:** Search historical observations and findings across sessions.
* **Syntax:**
  * `tm search "<query>"`
* **Validation Criteria:** Returns ranked matches with task ID, snippet, and timestamp.

---

## Phase 3: Session Lifecycle & Crash-Resilient Context Engine

### Slice 3.1: Session Initialization & Crash Recovery (`tm init`)
* **Objective:** Provide a single command that initializes or resumes a session, returning full project context.
* **Logic:**
  1. Check for any previous session that was not cleanly closed.
  2. If found, mark as `interrupted` and synthesize a recovery note from the last active task and checkpoint comment.
  3. Create a new active session.
  4. Fetch current active task, open blockers, top pending tasks, and recent observations.
  5. Print human summary or structured JSON (`--json`).
* **Validation Criteria:** Integration test where a process is killed mid-task; subsequent `tm init` recovers and reports the exact interrupted state.

### Slice 3.2: Optional Session Handoff (`tm handoff`)
* **Objective:** Allow agents or developers to record a clean handoff summary and declare the next planned action.
* **Syntax:**
  * `tm handoff "Completed unit tests" next:"Implement multiplication"`
* **Logic:**
  * Marks the current session `completed`.
  * Persists `summary` and `next_action`.
* **Validation Criteria:** If called, the next `tm init` reflects the clean handoff note; if omitted, `tm init` cleanly falls back to the last checkpoint without errors.

---

## Phase 4: Human TUI & Container Packaging

### Slice 4.1: Terminal UI Dashboard (`tm tui`)
* **Objective:** Provide a full-screen interactive terminal dashboard for developers using `charmbracelet/bubbletea` and `lipgloss`.
* **Deliverables:**
  * Task navigation (`j`/`k`, arrows).
  * Filter tabs (All, Pending, In Progress, Blocked, Completed).
  * Status toggle (mark done, start, block).
  * Observation viewer modal.
* **Validation Criteria:** Interactive TUI starts, renders cleanly on macOS terminal, responds to keys, and updates SQLite state.

### Slice 4.2: Build Automation & Static Binary Release
* **Objective:** Produce zero-dependency static binaries for macOS and Linux.
* **Deliverables:**
  * `Makefile` targets:
    * `make build`: builds `bin/tm` for host.
    * `make build-linux`: builds `bin/tm-linux-amd64` (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`).
    * `make test`: runs all unit and integration tests.
* **Validation Criteria:** Static Linux binary runs directly inside a minimal Docker container (`alpine` or `debian-slim`) with no libraries installed.

### Slice 4.3: Sandcastle & CLAUDE.md Integration
* **Objective:** Update project templates to instruct agents on using `tm` via bash.
* **Deliverables:**
  * Updated `CLAUDE.md` snippet for agent repositories.
  * Update Sandcastle runner to bind-mount the `tm` binary directly into containers.
* **Validation Criteria:** Claude Code runs inside a Sandcastle container and successfully executes `tm init --json` on turn 1.
