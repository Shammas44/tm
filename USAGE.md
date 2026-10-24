# TaskManager CLI (`tm`) — User Guide & Operational Manual

**TaskManager (`tm`)** is a zero-dependency, crash-resilient task and working-memory management CLI engine written in pure Go. It is designed to be used identically by **human developers** in their local terminal and by **autonomous AI coding agents** (e.g. Claude Code via Sandcastle) inside Docker containers.

---

## 1. Key Tenets & Architecture

* **Zero Runtime Dependencies:** Built using pure Go (`modernc.org/sqlite`, no CGO). A single static binary runs on any Linux distribution or macOS system with zero external libraries.
* **Taskwarrior-Inspired Ergonomics:** Fast, positional command shortcuts (`tm add ...`, `tm 1 start`, `tm 1 done`, `tm ls`).
* **Dual Output Format:**
  * **Interactive TTY:** Clean ANSI-colored tables, status indicators, and age metrics.
  * **Machine / Pipe / `--json`:** Deterministic, compact, token-efficient JSON.
* **Crash-Resilient State Engine:** `tm init` synthesizes project context and automatically recovers from unhandled agent interruptions or crashes using SQLite checkpoints.
* **Embedded SQLite with FTS5:** All task states, sessions, observations, and full-text search indexes reside in a single `state.db` database using WAL mode (`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`).

---

## 2. Installation & Build

### 2.1 Building from Source

Clone the repository and compile using `make`:

```bash
# Build native binary for your host (macOS or Linux)
make build

# The binary is placed at bin/tm
./bin/tm --help
```

### 2.2 Cross-Compiling Static Linux Binary (for Docker / Containers)

To produce a completely static, stripped ELF binary for container bind-mounting:

```bash
make build-linux
# Binary is generated at bin/tm-linux-amd64 (CGO_ENABLED=0, stripped, ~8MB)
```

### 2.3 Installing into `$PATH`

```bash
# Local machine
sudo cp bin/tm /usr/local/bin/tm

# Verify installation
tm --help
```

---

## 3. Quickstart Tutorial

### Step 1: Initialize your working session
At the start of your workday or turn, run `tm init`:
```bash
tm init
```
*Output:*
```text
=== TaskManager Session Initialized ===
Session ID: e1fbe63e-1fa3-4ba0-bd62-b85b5f5e560b
Target Task: No task currently active (0 pending tasks available).
Backlog: 0 pending, 0 in progress, 0 blocked, 0 completed
```

### Step 2: Add tasks to the backlog
```bash
# Standard syntax with inline Taskwarrior attributes
tm add "Implement division error handling" priority:H project:calc desc:"Verify divide-by-zero returns error code"
tm add "Write edge-case unit tests" priority:M project:calc
```

### Step 3: View tasks
```bash
tm list
# or using the shortcut alias:
tm ls
```
*Output:*
```text
ID    Status        Pri   Project       Title                                Age   
--------------------------------------------------------------------------------
1     pending       H     calc          Implement division error handling    12s   
2     pending       M     calc          Write edge-case unit tests           5s    
```

### Step 4: Start work on a task
```bash
tm 1 start
```
*Output:*
```text
Started task #1: Implement division error handling
```
Task `#1` is now marked `in_progress` and flagged as the active task in the session checkpoint.

### Step 5: Record technical findings & observations
Persist technical discoveries, memory invariants, or compiler findings directly linked to the task:
```bash
tm 1 log "Found that ASan detects heap-use-after-free on line 42 of parser.c"
```

### Step 6: Search knowledge base
Search across all task logs and observations using SQLite FTS5 BM25 ranking:
```bash
tm search "heap-use-after-free"
```
*Output:*
```text
ID     Task#   Age    Content                                                
--------------------------------------------------------------------------------
1      #1      45s    Found that ASan detects heap-use-after-free on line 42...
```

### Step 7: Block or complete tasks
If a task is blocked:
```bash
tm 1 block "Waiting on upstream PR #14"
```

When finished and verified:
```bash
tm 1 done
```

### Step 8: Clean session handoff (Optional)
Record a clean handoff summary and declare the next planned action:
```bash
tm handoff "Finished division error handling" next:"Implement unit tests for edge cases"
```

---

## 4. Database Location & Resolution Priority

`tm` stores all task state, working memory observations, sessions, and FTS5 search indexes in a single SQLite database file (`state.db`). You can define which database file to use in **4 ways**, evaluated in the following strict order of precedence:

### 1. Explicit CLI Flag (`--db <path>`) *(Highest Priority)*
Pass `--db` directly to any command. Accepts absolute or relative paths and overrides all other settings:
```bash
tm --db /path/to/my-state.db list
tm --db ./state.db init
tm --db /tmp/test.db 1 done
```

### 2. Environment Variable (`TASKMANAGER_DB`)
Set `TASKMANAGER_DB` in your terminal shell, `.env` file, CI/CD pipeline, or Docker container:
```bash
export TASKMANAGER_DB=/data/project-tasks.db
tm list
```
In Docker/container environments:
```yaml
environment:
  - TASKMANAGER_DB=/home/agent/.taskmanager/state.db
```

### 3. Project Context (`-P / --project <name>` or `TASKMANAGER_PROJECT`)
When managing multiple projects on a developer machine, you can specify a project slug. `tm` will automatically place and resolve the database at `~/.taskmanager/projects/<project-slug>/state.db`:
```bash
# Uses ~/.taskmanager/projects/calc/state.db
tm -P calc list

# Or via environment variable
export TASKMANAGER_PROJECT=my-app
tm init
```

### 4. Automatic Local Fallback *(Zero-Config Default)*
If none of the above are defined:
1. `tm` checks if a `state.db` file already exists in the **current working directory** (`./state.db`). If present, it uses it.
2. Otherwise, `tm` defaults to **`./.taskmanager/state.db`** (automatically creating the `.taskmanager/` directory if needed).

---

## 5. Command-Line Reference

### Global Flags

| Flag | Description | Default |
| :--- | :--- | :--- |
| `--db <path>` | Path to SQLite database file | `~/.taskmanager/projects/<project>/state.db` or `./.taskmanager/state.db` or `./state.db` |
| `--json` | Force raw, compact JSON output | `false` (auto-detected when piped) |
| `-q, --quiet` | Suppress non-essential output | `false` |
| `-P, --project <name>`| Filter commands by project slug | `""` (all projects) |

---

### `tm init`
Initializes a new session, auto-detects crashed or interrupted sessions, and restores working memory.

* **Usage:** `tm init [--json] [--agent <id>]`
* **JSON Output Schema:**
  ```json
  {
    "session_id": "c7a8e120-d3b0-477b-8c03-4019dc61a020",
    "resumed_from_interrupt": false,
    "last_handoff": "Implemented calc_divide, need to write edge-case unit tests.",
    "active_task": {
      "id": 1,
      "title": "Implement division error handling",
      "status": "in_progress",
      "priority": "H",
      "project": "calc",
      "active": true
    },
    "blockers": [],
    "recent_observations": [
      "Found that ASan detects heap-use-after-free on line 42 of parser.c"
    ],
    "backlog": {
      "pending": 3,
      "in_progress": 1,
      "blocked": 0,
      "completed": 5
    }
  }
  ```

---

### `tm add` / `tm new`
Creates a new task in the backlog.

* **Usage:** `tm add "<title>" [priority:H|M|L|C] [project:<name>] [desc:"<description>"]`
* **Aliases:** `tm new`
* **Examples:**
  ```bash
  tm add "Fix race condition in pool" priority:C project:core
  tm add "Update API docs" priority:L
  ```

---

### `tm list` / `tm ls`
Lists tasks with optional status and priority filters.

* **Usage:** `tm list [status:pending|in_progress|blocked|completed] [priority:H|M|L|C] [project:<slug>] [--all] [--json]`
* **Examples:**
  ```bash
  tm ls
  tm ls in_progress
  tm ls priority:H
  tm ls --all
  ```

---

### Task State Mutations (`tm <id> <action>`)

Fast positional commands for instant task state updates:

* **Start task:** `tm <id> start`  
  Sets status to `in_progress`, marks `active = 1`, and unsets active on all other tasks.
* **Complete task:** `tm <id> done`  
  Sets status to `completed`, sets `completed_at`, unsets active, and records a completion observation.
* **Block task:** `tm <id> block "<reason>"`  
  Sets status to `blocked`, records blocker reason, and unsets active.
* **Log note:** `tm <id> log "<note>"`  
  Appends an observation to `<id>`, indexes it into FTS5, and updates the session checkpoint.
* **Inspect task:** `tm <id>` or `tm show <id>`  
  Shows complete task details, age, blocker reasons, and description.
* **Cancel task:** `tm delete <id>` (aliases: `del`, `rm`)  
  Marks task status as `cancelled`.

---

### `tm search`
Performs BM25-ranked full-text search across all notes, discoveries, and observations.

* **Usage:** `tm search "<query>" [--json]`
* **Example:**
  ```bash
  tm search "AddressSanitizer"
  ```

---

### `tm handoff` (Optional)
Concludes the active session cleanly and declares next steps.

* **Usage:** `tm handoff "<summary>" [next:"<action>"] [--json]`
* **Example:**
  ```bash
  tm handoff "Completed parser unit tests" next:"Implement AST evaluator"
  ```
> [!NOTE]
> If an agent fails to call `tm handoff` (e.g. process killed, timeout, token exhaustion), `tm init` automatically recovers the in-progress task and latest checkpoint without data loss or deadlocks.

---

### `tm tui`
Full-screen interactive terminal dashboard built with Bubbletea and Lipgloss with an experience inspired by Taskwarrior and `vit`.

* **Launch:** `tm tui`
* **Keyboard Navigation & Actions:**
  * `j` / `k` or `Down` / `Up`: Navigate task list
  * `a` or `c`: **Create new task** using your `$EDITOR` (vim, nano, etc.)
  * `e`: **Edit selected task** in `$EDITOR` (modifies title, status, priority, project, description, blocker reason)
  * `Tab` / `Shift+Tab`: Switch tabs (`All`, `Pending`, `In Progress`, `Blocked`, `Completed`)
  * `Enter`: Start task (`in_progress`)
  * `x`: Mark task completed (`done`)
  * `b`: Open prompt to enter blocker reason
  * `v`: Open observation viewer modal for the selected task
  * `/`: Search and filter tasks by title
  * `q` or `Ctrl+C`: Exit TUI

#### Editor Integration (`$EDITOR` / `vit`-Style Editing):
When pressing `a` (create) or `e` (edit), `tm tui` temporarily suspends the terminal and opens your configured editor (`$EDITOR` or `$VISUAL`, defaulting to `vim` / `nano` / `vi`). A structured template is populated:

```text
# TaskManager: Edit Task #3
# Lines starting with '#' are comments and will be ignored.
# Save and close the editor to commit your modifications.
# Valid Statuses: pending, in_progress, blocked, completed, cancelled
# Valid Priorities: L, M, H, C

Title: Implement multiplication error handling
Status: in_progress
Priority: H
Project: calc
Blocker Reason: 

# Everything below 'Description:' will be treated as the task description.
Description:
Verify divide-by-zero returns error code and test with ASan.
```
Saving and closing your editor immediately validates and commits the changes into SQLite and refreshes the TUI dashboard. Exiting without a title cancels the operation.

---

## 6. Autonomous AI Agent & Docker Integration

### 5.1 Docker Bind-Mount Architecture

Because `tm-linux-amd64` is a zero-dependency static binary, it can be mounted directly into any minimal container (Alpine, Ubuntu, Debian):

```text
┌─────────────────────────────────┐
│ Host Machine (macOS / Linux)    │
│   • bin/tm-linux-amd64          │
│   • ~/.taskmanager/projects/app │
└──────────────┬──────────────────┘
               │ Docker bind-mounts
               ▼
┌─────────────────────────────────┐
│ Container (/usr/local/bin/tm)   │
│                                 │
│ AI Agent executes standard bash:│
│   1. tm init --json             │
│   2. tm 1 start                 │
│   3. cmake && ctest             │
│   4. tm 1 log "notes..."        │
│   5. tm 1 done                  │
│   6. tm handoff "..."           │
└─────────────────────────────────┘
```

### 5.2 Sandcastle Runner Configuration (`.sandcastle/main.ts`)

```typescript
import { createSandbox, claudeCode } from "@ai-hero/sandcastle";
import { docker } from "@ai-hero/sandcastle/sandboxes/docker";
import { resolve, join } from "node:path";
import { homedir } from "node:os";

const projectSlug = "c-dev";
const hostProjectDir = join(homedir(), ".taskmanager", "projects", projectSlug);
const tmBinaryPath = resolve("./bin/tm-linux-amd64");

await using sandbox = await createSandbox({
  sandbox: docker({
    imageName: "sandcastle:c-dev",
    mounts: [
      // Bind-mount the static binary directly into container $PATH
      {
        hostPath: tmBinaryPath,
        sandboxPath: "/usr/local/bin/tm",
        readonly: true,
      },
      // Bind-mount SQLite state directory
      {
        hostPath: hostProjectDir,
        sandboxPath: `/home/agent/.taskmanager/projects/${projectSlug}`,
      },
    ],
  }),
});
```

### 5.3 Agent System Instructions (`CLAUDE.md`)

```markdown
# Agent Execution Instructions

1. Run `tm init --json` on turn 1 to retrieve current active task, blockers, and recovery state.
2. If `active_task` is set, resume work immediately. If not, inspect backlog with `tm list --json` and start the next task with `tm <id> start`.
3. Verify your work using automated tests and compiler diagnostics.
4. Record key technical discoveries: `tm <id> log "<finding>"`.
5. Complete task: `tm <id> done`.
6. Conclude cleanly: `tm handoff "<summary>" next:"<next-action>"`.
```
