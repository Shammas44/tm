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

### Task State Mutations & CLI Operations (`tm <id> <action>` or `tm <action> <id>`)

Fast CLI commands providing full parity with TUI operations:

* **Start task:** `tm <id> start` (or `tm start <id>`)  
  Sets status to `in_progress`, marks `active = 1`, and unsets active on all other tasks.
* **Complete task:** `tm <id> done` (or `tm done <id>`)  
  Sets status to `completed`, sets `completed_at`, unsets active, and records a completion observation.
* **Block task:** `tm <id> block "<reason>"` (or `tm block <id> "<reason>"`)  
  Sets status to `blocked`, records blocker reason, and unsets active.
* **Log note:** `tm <id> log "<note>"` (or `tm log <id> "<note>"`)  
  Appends an observation to `<id>`, indexes it into FTS5, and updates the session checkpoint.
* **Edit task:** `tm edit <id> [flags]` (or `tm <id> edit`)  
  - Non-interactive flag-based editing: `--title "..."`, `--status <status>`, `--priority <prio>`, `--project <proj>`, `--desc "..."`, `--tags <t1,t2>`, `--depends <d1,d2>`, `--block-reason "..."`.
  - Interactive editing: When run without modifier flags in a terminal, opens `$EDITOR` with the vit/Taskwarrior template to edit fields and modify/delete past observation notes.
* **Tag manipulation:** `tm tag <id> [+tag|-tag...]` (or `tm <id> tag +tag -tag`)  
  Add (`+tag`) or remove (`-tag`) tags on an existing task.
* **Dependency management:** `tm dep <id> [+depID|-depID...]` (or `tm <id> dep 2`)  
  Add (`+depID`) or remove (`-depID`) prerequisite task dependencies.
* **Inspect task:** `tm <id>` or `tm show <id>`  
  Shows complete task details: status, priority, project, user tags, synthetic virtual tags (`READY`, `BLOCKED`, etc.), prerequisite dependencies with their current statuses, blocker reason, description, and full observation history.
* **Cancel task:** `tm delete <id>` (aliases: `del`, `rm`, `tm <id> del`)  
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
Full-screen interactive terminal dashboard built with Bubbletea and Lipgloss with an experience inspired by Taskwarrior, `vit`, and Vim.

* **Launch:** `tm tui`
* **Column Layout (Pixel-Perfect Visual Alignment & UX):**
  - `ID`: Task auto-increment identifier
  - `Status`: Current status (`pending`, `in_progress`, `blocked`, `completed`, `cancelled`) with active indicator (`in_prog*`)
  - `Pri`: Priority (`C`, `H`, `M`, `L`)
  - `Project`: Project slug
  - `Tags`: User-defined labels (truncated with `...` if exceeding column width)
  - `Deps`: Prerequisite task dependencies
  - `Notes`: Number of technical observations/notes recorded for this task
  - `Created`: Human-readable timestamp formatted as `YYYY-MM-DD HH:MM` (e.g. `2026-10-24 14:15`)
  - `Title`: Task description/title (truncated with `...` if exceeding available terminal width)

* **Visual Grid Selection & Interactive Column Sorting:**
  - **Crosshair Selection:** Navigate rows with `j`/`k` (or `Down`/`Up`) and select columns with `h`/`l` (or `Left`/`Right`). The currently selected cell and column header are distinctly highlighted.
  - **Column Sorting (`s`):** Pressing `s` sorts by the currently selected column. Pressing `s` again toggles sort direction between ascending (`▲`) and descending (`▼`).
  - **Continuous Backlog Browsing:** No artificial pagination; smoothly browse all tasks continuously with viewport scrolling.

* **Vim-Style Navigation & Keyboard Actions:**
  - `j` / `k` or `Down` / `Up`: Navigate rows down / up
  - `h` / `l` or `Left` / `Right`: Select column left / right
  - `J` / `K` or `Ctrl+D` / `Ctrl+U`: Scroll task details inspection pane down / up
  - `gg`: Jump to top of task list (first task)
  - `G` or `End`: Jump to bottom of task list (last task)
  - `PgUp` / `PgDown`: Scroll task list by page
  - `Tab` / `Shift+Tab`: Cycle through categories (`All`, `Ready`, `Pending`, `In Progress`, `Blocked`, `Completed`)
  - `1` - `6`: Jump directly to category
  - `s`: Sort by selected column (press again to reverse direction)
  - `a` or `c`: **Create new task** using `$EDITOR`
  - `e`: **Edit selected task** in `$EDITOR` (or `:edit [id]`)
  - `Enter`: Start task (`in_progress` & `active`)
  - `x`: Mark task completed (`done`)
  - `b`: Prompt for blocker reason
  - `d`: **Delete task permanently** (prompts for confirmation; blocked if other tasks depend on it)
  - `/`: Enhanced search & filter (`+tag`, `+READY`, `+BLOCKED`, `project:x`, `pri:H`, text)
  - `:`: **Enter Vim Command Mode** (`:add`, `:start`, `:done`, `:block`, `:log`, `:tag`, `:dep`, `:edit`, `:del`, `:q`)
  - `?`: **Open complete full-screen controls & shortcuts help window**
  - `q` or `Ctrl+C`: Exit TUI

* **Dual-Column Readonly Inspection Pane (Left/Right Split):**
  - The bottom half of the screen is divided into two side-by-side columns:
    - **Left Column (Description & Prerequisites):** Displays task description and incomplete prerequisite dependencies.
    - **Right Column (Observations & Blockers):** Displays observations using clean bullet points (`• [date] (#id)`), with any blocker reason prominently featured as the first observation item (`• [BLOCKER] Blocker Reason:`).
  - Maximizes vertical space with zero empty newline padding.
  - Scrollable directly with `J` / `K` or `Ctrl+D` / `Ctrl+U`.

* **Command Mode (`:`):**
  - Pressing `:` activates the bottom command prompt:
    - `:q` / `:quit` / `:exit`: Exit TUI
    - `:add <title>`: Create a new task
    - `:start [id]`: Start task (defaults to selected task)
    - `:done [id]`: Mark task completed
    - `:block [id] <reason>`: Block task with reason
    - `:log [id] <note>`: Append observation note to task
    - `:tag [id] +tag -tag`: Add or remove tags
    - `:dep [id] +depID -depID`: Add or remove dependencies
    - `:edit [id]`: Open task in `$EDITOR`
    - `:del [id]`: Delete task
    - `:help`: Open full help window

#### Editor Integration (`$EDITOR` / YAML-Formatted Editing):
When pressing `a` (create) or `e` (edit), `tm tui` suspends the terminal and opens `$EDITOR` (or `$VISUAL`, defaulting to `vim` / `nano` / `vi`) with a temporary `.yaml` file for full syntax highlighting.

In edit mode (`e`), the editor displays **read-only task metadata comments** at the top, followed by YAML fields for task attributes, multi-line blocker reason, existing notes, new note, and multi-line description:

```yaml
# ==============================================================================
# READ-ONLY TASK METADATA: Task #1
# Created: 2026-10-24 10:18:28 | Updated: 2026-10-24 11:02:15
# Virtual Tags: ACTIVE, READY, UNBLOCKED
# Dependencies: #2 (completed)
# ==============================================================================
# EDITABLE TASK DEFINITION (Save and close editor to commit changes)
# Valid Statuses: pending, in_progress, blocked, completed, cancelled
# Valid Priorities: L, M, H, C
# Multi-line fields (blocker_reason, description, notes): use '|-' format
# ==============================================================================

title: Implement multiplication error handling
status: in_progress
priority: H
project: calc
tags:
  - math
  - compiler
depends:
  - 2
blocker_reason: |-
  Waiting on float precision verification.
  Blocked until libm test suite passes.
new_note: |-
  Found that IEEE 754 overflow traps can be avoided with flag checks.
notes:
  1: ASan detects heap-use-after-free in parser line 42
  2: |-
    Completed reproduction unit test in tests/test_parser.c
    Verified fix on Linux kernel 6.8.
description: |-
  Verify divide-by-zero returns error code and test with ASan.
  Ensure zero-allocation error path.
```

Saving and closing the editor immediately updates the task:
- Modifying the text of a note in the `notes:` map updates the observation in SQLite and the FTS5 search index.
- Removing an entry from `notes:` deletes that observation.
- Specifying `blocker_reason:` supports multi-line text (via block scalars `|-`) rendered cleanly in the TUI readonly pane.
- Specifying `new_note:` records a new observation timestamped to the active session.
- Exiting with an empty title cancels the operation.

#### Virtual Tags (Taskwarrior-Style):
`tm` dynamically computes synthetic virtual tags based on task dependencies and state:
* `READY`: Pending tasks whose prerequisites are all completed and have no blockers.
* `ACTIVE`: Currently active / in-progress tasks.
* `BLOCKED`: Tasks blocked by a reason or by incomplete prerequisite dependencies.
* `UNBLOCKED`: Tasks with zero blocking dependencies.
* `BLOCKING`: Incomplete tasks that another task depends on.
* `HAS_NOTE`: Tasks with recorded observations or descriptions.
* `TODAY`: Tasks created today.
* `PRI_CRIT` / `PRI_HIGH` / `PRI_MED` / `PRI_LOW`: Priority tags.

In the search prompt (`/`), you can filter with `+READY`, `+BLOCKED`, `+tag`, or exclude with `-tag`.

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
