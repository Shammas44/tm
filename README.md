# TaskManager CLI (`tm`)

**TaskManager (`tm`)** is a zero-dependency, single-binary CLI task and working-memory management engine written in pure Go. It provides persistent execution context, context-reset recovery, and task governance for both **human software engineers** in their local terminals and **autonomous AI coding agents** (e.g. Claude Code via Sandcastle) inside Docker containers.

---

## Key Highlights

* **Zero External Dependencies:** Built with pure Go (`modernc.org/sqlite`, no CGO). Compiles to a single static binary that runs anywhere without external packages or daemons.
* **Taskwarrior Ergonomics:** Fast, positional commands (`tm add ...`, `tm 1 start`, `tm 1 done`, `tm ls`).
* **Dual Output Modes:** Formatted ANSI tables for interactive TTY sessions; deterministic, compact JSON for `--json` or automated subshell pipelines.
* **Crash-Resilient State Recovery:** `tm init` automatically recovers in-progress tasks and notes from unclosed or interrupted sessions.
* **SQLite with FTS5:** All tasks, sessions, observations, and BM25 full-text search indexes live in an embedded `state.db` file using WAL mode.
* **Human Terminal Dashboard:** Interactive full-screen Bubbletea TUI via `tm tui`.

---

## Defining Which Database to Use

`tm` evaluates the target database in **4 steps of precedence**:

1. **CLI Flag (`--db <path>`)** *(Highest Priority)*:
   ```bash
   tm --db /path/to/my-state.db list
   ```
2. **Environment Variable (`TASKMANAGER_DB`)**:
   ```bash
   export TASKMANAGER_DB=/data/state.db
   tm list
   ```
3. **Project Context (`-P / --project <name>` or `TASKMANAGER_PROJECT`)**:
   Resolves automatically to `~/.taskmanager/projects/<name>/state.db`:
   ```bash
   tm -P calc list
   # or
   export TASKMANAGER_PROJECT=calc
   tm list
   ```
4. **Local Fallback *(Zero-Config Default)***:
   * Uses `./state.db` if it already exists in the current directory.
   * Otherwise defaults to `./.taskmanager/state.db` (auto-creating `.taskmanager/` if needed).

---

## Quickstart

```bash
# 1. Build binaries
make build         # Native binary at bin/tm
make build-linux   # Static Linux binary at bin/tm-linux-amd64 (CGO_ENABLED=0)

# 2. Initialize session
./bin/tm init

# 3. Add tasks
./bin/tm add "Implement division error codes" priority:H project:calc desc:"Verify divide-by-zero"
./bin/tm add "Write unit tests" priority:M project:calc

# 4. View backlog
./bin/tm list

# 5. Start and mutate tasks
./bin/tm 1 start
./bin/tm 1 log "Checking IEEE 754 division-by-zero flags"
./bin/tm search "division"
./bin/tm 1 done

# 6. Record clean handoff
./bin/tm handoff "Completed division error codes" next:"Write unit tests"

# 7. Launch interactive terminal UI
./bin/tm tui
```

---

## Documentation Index

* 📖 [**User Guide & Operational Manual (`USAGE.md`)**](USAGE.md)
* 📘 [**Technical Specification (`SPECIFICATION.md`)**](SPECIFICATION.md)
* 🗺️ [**Implementation Roadmap (`ROADMAP.md`)**](ROADMAP.md)
* ⏱️ [**Implementation Progress Tracker (`PROGRESS.md`)**](PROGRESS.md)
