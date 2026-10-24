# TaskManager CLI (`tm`) — Technical Specification

## 1. Overview & Goals

**TaskManager CLI (`tm`)** is a zero-dependency, single-binary task and session management tool written in pure Go. It is designed to be used identically by **human developers** in their local terminals and by **AI coding agents** (e.g. Claude Code via Sandcastle) inside Docker containers.

### Key Tenets
1. **Zero Runtime Dependencies**: Built with pure Go (`modernc.org/sqlite`, no CGO). A single static binary runs anywhere without external libraries, Taskwarrior installations, or Python virtual environments.
2. **Taskwarrior-Inspired Ergonomics**: Short, positional commands (`tm add ...`, `tm 1 start`, `tm 1 done`, `tm ls`).
3. **Dual Output Mode**:
   - When run in an interactive terminal (TTY): clean, ANSI-styled tables, status indicators, and human-friendly summaries.
   - When run with `--json` or redirected to a pipe/file: deterministic, compact, token-efficient JSON.
4. **Crash-Resilient State Engine**: `tm init` provides the agent with immediate project context (active task, blockers, recent observations, and recovery state). `tm handoff` is optional; every task update automatically persists a recovery checkpoint so no state is lost if an agent is abruptly interrupted.
5. **Single SQLite Database**: All data (tasks, sessions, observations, FTS5 full-text index) lives in a single SQLite file (`state.db`) configured with `PRAGMA journal_mode=WAL` and `PRAGMA busy_timeout=5000`.

---

## 2. Command-Line Interface Specification

The binary is named `tm` (with `taskmanager` as a symlink/alias).

### Global Flags
* `--db <path>`: Path to the SQLite database file. Resolution order:
  1. `--db <path>` explicit flag.
  2. `TASKMANAGER_DB` environment variable.
  3. `-P / --project <name>` or `TASKMANAGER_PROJECT` -> `~/.taskmanager/projects/<project-slug>/state.db`.
  4. Local fallback: `./state.db` if present in current directory, otherwise `./.taskmanager/state.db`.
* `--json`: Forces raw JSON output on `stdout` regardless of whether the output is a TTY.
* `-q, --quiet`: Suppresses informational messages and prints only essential IDs or data.
* `-P, --project <slug>`: Project slug filter or target context.

---

### Command 1: `tm init`
Called at the beginning of an agent turn or when a developer sits down to resume work.

* **Usage**: `tm init [--json] [--agent <id>]`
* **Behavior**:
  1. Checks for any active session in the database.
  2. If the previous session was not cleanly closed with `tm handoff`, marks it as `interrupted` and constructs a resumption summary from the last active task and the latest logged note.
  3. Opens a new active session.
  4. Returns the current active task (or top priority pending task), any open blockers, recent notes, and backlog counts.
* **JSON Output Schema**:
  ```json
  {
    "session_id": "c7a8e120-...",
    "resumed_from_interrupt": false,
    "last_handoff": "Implemented calc_divide, need to write edge-case unit tests.",
    "active_task": {
      "id": 3,
      "title": "Add unit tests for divide-by-zero",
      "status": "started",
      "priority": "H",
      "project": "c-dev"
    },
    "blockers": [],
    "recent_observations": [
      "CMake requires Ninja 1.11+ for sanitizers"
    ],
    "backlog": {
      "pending": 4,
      "started": 1,
      "blocked": 0,
      "completed": 6
    }
  }
  ```

---

### Command 2: `tm add`
Creates a new task.

* **Usage**: `tm add "<title>" [priority:H|M|L|C] [project:<name>] [desc:"<description>"]`
* **Aliases**: `tm new`
* **Examples**:
  ```bash
  tm add "Implement division error codes" priority:H project:calc
  tm add "Review memory leak findings" priority:M
  ```
* **JSON Output**: Returns the created task object.

---

### Command 3: `tm list` / `tm ls`
Lists tasks with optional filtering.

* **Usage**: `tm list [status:pending|started|blocked|completed] [priority:H|M|L|C] [project:<slug>] [--json]`
* **Human Output**: Concise ASCII/ANSI table with columns: `ID`, `Status`, `Pri`, `Project`, `Title`, `Age`.
* **JSON Output**: Array of task objects.

---

### Command 4: Task State Transitions (`tm <id> <action>`)
Positional commands to update task status quickly.

#### `tm <id> start`
Marks task `<id>` as `started` and flags it as the current `active` task.
```bash
tm 3 start
```

#### `tm <id> done`
Marks task `<id>` as `completed` with `completed_at = CURRENT_TIMESTAMP`, clears active flag, and records a completion observation.
```bash
tm 3 done
```

#### `tm <id> block "<reason>"`
Marks task `<id>` as `blocked`, records the blocker reason, and disengages the active task flag.
```bash
tm 3 block "Waiting on upstream PR #14"
```

#### `tm <id> log "<note>"`
Attaches an observation/note to task `<id>`, stores it in SQLite, and indexes it into the FTS5 table. Also automatically updates the current session's checkpoint.
```bash
tm 3 log "Found that ASan detects heap-use-after-free in parser line 42"
```

---

### Command 5: `tm search`
Full-text search across all historical task logs, observations, and findings.

* **Usage**: `tm search "<query>" [--json]`
* **Behavior**: Executes SQLite FTS5 query with BM25 ranking.
* **JSON Output**: Array of matching observations with task ID, timestamp, and snippet.

---

### Command 6: `tm handoff` (Optional)
Cleanly concludes the current session, supporting both **plain text** and **structured JSON**.

* **Usage**: `tm handoff [note or JSON] [--file <path>] [--json]`
* **Behavior**:
  1. Marks the active session as `completed`.
  2. Saves polymorphic `handoff` payload (parsed dynamically as typed JSON object or plain text).
* **Note**: If the agent does not call this (e.g. process terminated, timeout, token limit), `tm init` automatically handles recovery from the last checkpoint without crashing or deadlocking.

---

### Command 7: `tm session` / `tm sessions`
Inspect past sessions, audit interrupted runs, and view historical handoffs, observations, and tasks touched.

* **Usage**:
  - `tm session [list] [--limit <n>] [--json]`: Chronological table of sessions with duration, status, and handoff.
  - `tm session show <id>` (or `tm session <id|prefix>`): Detailed session inspection with prefix matching, observations recorded in that session, and tasks touched.

---

### Command 8: `tm tui`
Interactive full-screen terminal dashboard for developers with a `vit`-inspired editor workflow.

* **Built with**: `charmbracelet/bubbletea` and `lipgloss`.
* **Features**:
  * Keyboard navigation (`j`/`k`, arrow keys).
  * External editor integration via `$EDITOR` / `$VISUAL` (defaults to `vim` / `nano` / `vi`):
    * `a` or `c`: Creates a new task in editor using structured template.
    * `e`: Edits the selected task (title, status, priority, project, description, blocker reason).
  * Filter tabs: All, Pending, In Progress, Blocked, Done.
  * Instant status toggle (`Enter` to start, `x` to mark done, `b` to block).
  * Observation viewer modal (`v`).
  * Search (`/`).

---

## 3. Database Schema (`state.db`)

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL CHECK(status IN ('pending', 'started', 'in_progress', 'completed', 'blocked', 'cancelled')),
    priority TEXT NOT NULL CHECK(priority IN ('L', 'M', 'H', 'C')),
    project TEXT DEFAULT '',
    active INTEGER DEFAULT 0,
    blocker TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    agent_id TEXT DEFAULT 'developer',
    active_task_id INTEGER REFERENCES tasks(id),
    status TEXT NOT NULL CHECK(status IN ('active', 'completed', 'interrupted')),
    checkpoint TEXT DEFAULT '',
    handoff TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    next_action TEXT DEFAULT '',
    last_checkpoint_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at DESC);

CREATE TABLE IF NOT EXISTS observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT REFERENCES sessions(id),
    task_id INTEGER REFERENCES tasks(id),
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    content,
    content='observations',
    content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS observations_ai AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts(rowid, content) VALUES (new.id, new.content);
END;
```

---

## 4. Directory & Code Structure

```text
taskmanager-go-worktree/
├── cmd/
│   └── tm/
│       └── main.go              # CLI entrypoint (routes to cobra commands)
├── internal/
│   ├── cmd/                     # Cobra command definitions (add, list, init, handoff, tui, etc.)
│   ├── db/                      # SQLite driver setup, WAL config, schema migrations
│   ├── model/                   # Domain structs (Task, Session, Observation)
│   ├── store/                   # SQLite data access methods (TaskStore, SessionStore, FTS5)
│   ├── output/                  # Dual formatter (TTY table printer + compact JSON serializer)
│   └── tui/                     # Bubbletea interactive dashboard
├── Makefile                     # Build targets (host build, static Linux build, test)
├── go.mod
├── go.sum
├── SPECIFICATION.md
├── ROADMAP.md
└── handoff.json
```
