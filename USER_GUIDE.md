# TaskManager MCP Server: User Guide & Operational Manual

This guide walks you through setting up, configuring, and using the **TaskManager MCP Server** to provide persistent working memory, context-reset recovery, and workflow governance for autonomous AI coding agents (such as Claude Desktop, Claude Code, Cursor, Devin, or custom agent harnesses).

---

## 1. What is TaskManager MCP?

Large Language Model (LLM) agents operating on codebases frequently suffer from:
* **Context Reset Amnesia:** When the agent's context window resets, compacted summaries lose task dependencies, unresolved blockers, and technical discoveries.
* **Ungoverned Execution:** Agents often make premature code edits without planning, skip verification, or fail to document findings.
* **Race Conditions on Task IDs:** Direct CLI exposure of tools like Taskwarrior causes ID recalculation errors.

The **TaskManager MCP Server** acts as an authoritative execution-state manager. Operating over **standard input/output (`stdio`) transport**, it exposes high-level semantic tools backed by:
1. **Isolated Taskwarrior Databases:** Scheduling, dependencies, priorities, and immutable UUIDv4 identifiers.
2. **SQLite with FTS5:** Sessions, blockers, append-only audit trail, and durable engineering observations indexed with full-text search (BM25 ranking).
3. **Deterministic Invariant Engine:** Enforces engineering discipline (e.g. proof of verification before completing tasks, clean exit requirements).

---

## 2. System Requirements & Prerequisites

* **Operating System:** macOS or Linux
* **Python:** Version 3.12 or newer (Python 3.14 fully supported)
* **Taskwarrior:** Version 3.0+ installed and available in `$PATH`
  ```bash
  # On macOS via Homebrew:
  brew install task
  
  # Verify installation:
  task --version
  ```

---

## 3. Installation

Clone the repository and install the package into a virtual environment:

```bash
cd repo

# Create a virtual environment
python3 -m venv .venv
source .venv/bin/activate

# Install taskmanager-mcp in editable mode with development dependencies
pip install -e ".[dev]"

# Verify installation
taskmanager-mcp --help
```

---

## 4. Authentication & Generating Access Tokens

The server operates over `stdio` transport and enforces **Ed25519 asymmetric cryptography**. 

A developer helper script is provided to automatically initialize signing keys in `~/.taskmanager/keys/` and mint scoped access tokens:

```bash
# Mint a token for agent 'agent-claude' working on project 'compiler' with 8 hours validity
python3 scripts/generate_dev_token.py --project compiler --agent agent-claude --role worker --ttl 480
```

### Script Output:
1. **Raw JWT Token:** Ready for environment variable configuration.
2. **Public Key Path:** Automatically set to `~/.taskmanager/keys/public.pem`.
3. **Ready-to-Paste Configuration Block:** Formatted for Claude Desktop or Cursor.

---

## 5. Integrating with AI Hosts

### 5.1 Claude Desktop Configuration
Open your Claude Desktop configuration file:
* **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
* **Linux:** `~/.config/Claude/claude_desktop_config.json`

Add the `taskmanager` server entry:

```json
{
  "mcpServers": {
    "taskmanager": {
      "command": "/ABSOLUTE/PATH/TO/.venv/bin/python3",
      "args": [
        "-m",
        "taskmanager_mcp",
        "--projects-root",
        "/Users/YOUR_USER/.taskmanager/projects",
        "--public-key-path",
        "/Users/YOUR_USER/.taskmanager/keys/public.pem",
        "--log-level",
        "INFO"
      ],
      "env": {
        "TASKMANAGER_AUTH_TOKEN": "YOUR_MINTED_JWT_TOKEN_HERE"
      }
    }
  }
}
```

> [!NOTE]
> Ensure you replace `/ABSOLUTE/PATH/TO/.venv/bin/python3` and `/Users/YOUR_USER` with your actual system paths.

### 5.2 Cursor Configuration
In Cursor's **Features > MCP Servers**, add a new stdio server:
* **Name:** `taskmanager`
* **Type:** `command`
* **Command:** `/ABSOLUTE/PATH/TO/.venv/bin/python3 -m taskmanager_mcp --projects-root /Users/YOUR_USER/.taskmanager/projects --public-key-path /Users/YOUR_USER/.taskmanager/keys/public.pem`
* **Environment Variables:**
  * `TASKMANAGER_AUTH_TOKEN`: `<YOUR_MINTED_JWT_TOKEN>`

---

## 6. Standard Agent Workflow & Available Tools

The MCP server enforces a disciplined 7-step software engineering lifecycle:

```
1. Start Session  ──►  2. Pick Next Task  ──►  3. Execute & Implement
       │                                                   │
       ▼                                                   ▼
7. End Session    ◄──  6. Reflect (Gate)   ◄──  5. Complete Task (Evidence)
                                                   ▲
                                                   │
                                        4. Record Observations
```

### 6.1 Session Management
* `session_begin(project, agent_id)`: Opens an active working session, restores prior session context, and retrieves active tasks and blockers.
* `session_heartbeat(session_id)`: Sends a periodic keepalive. Prevents the session from being marked as a zombie during long builds or test runs.
* `session_end(session_id, next_action)`: Safely closes the session. **Fails** if an active task was left unfinished without pausing, or if pending work remains without a concrete `next_action`.

### 6.2 Task Execution & Scheduling
* `task_get_next(component)`: Selects the single highest-urgency task whose prerequisites are completed and has no active blockers.
* `task_create(title, component, priority, dependencies, estimate_minutes, confidence, tags)`: Registers a new task. Verifies that the dependency graph remains acyclic (no circular dependencies).
* `task_create_subtask(parent_task_uuid, title, estimate_minutes)`: Creates a child task and automatically marks the parent as dependent on the child.
* `task_start(task_uuid)`: Sets the task as `ACTIVE` for the current session.
* `task_complete(task_uuid, summary, evidence, confidence)`: Marks task complete. **Mandatory requirements:**
  * `summary`: Minimum 20 characters explaining the resolution.
  * `evidence`: Minimum 10 characters of concrete verification (e.g. test outputs, command transcripts, diff hashes).
* `task_annotate(task_uuid, summary)`: Attaches structured incremental progress notes to an active task.
* `task_reprioritize(task_uuid, priority)`: Updates priority (`H`, `M`, `L`, `NONE`).
* `task_get(task_uuid)`: Retrieves full task metadata.
* `task_list(status, component, tags)`: Lists tasks matching criteria.

### 6.3 Knowledge Persistence (Observations)
* `observation_record(topic, content, component, confidence, code_references)`: Records durable engineering discoveries (e.g., architectural quirks, cache invalidation rules) into SQLite.
* `observation_search(query, component, min_confidence, limit)`: Performs fast full-text search with BM25 ranking across all recorded observations.

### 6.4 Blocker Management
* `blocker_create(task_uuid, reason, severity, blocking_task_uuid)`: Registers an issue blocking a task. Automatically transitions the affected task to `BLOCKED`. Blocked tasks are never scheduled by `task_get_next`.
* `blocker_resolve(blocker_id, resolution_summary)`: Resolves the blocker and unblocks the affected task.

### 6.5 Workflow Governance & Reflection
* `workflow_reflect(summary, stale_tasks_identified, recommendations)`: Conducts a retrospective. After every $N$ completed tasks (default: 5), `task_get_next` will return `REFLECTION_REQUIRED` until this tool is called to review technical debt and prune obsolete tasks.

---

## 7. Workflow Invariant Rules & Error Handling

When an agent attempts an invalid workflow operation, the server returns a structured error object with a descriptive message and an actionable **remedy**:

| Error Code | Violation | Actionable Remedy |
|---|---|---|
| `UNAUTHENTICATED` | Token missing or expired | Re-mint token using `scripts/generate_dev_token.py`. |
| `PROJECT_MISMATCH` | Token project claim mismatch | Use a token minted specifically for the target project. |
| `TASK_NOT_ACTIVE` | Rule 1: Attempted modification/completion of unstarted task | Invoke `task_start(task_uuid)` before making modifications. |
| `COMPLETION_EVIDENCE_MISSING`| Rule 2: Evidence < 10 chars or summary < 20 chars | Run test suite, capture output, and include in `evidence`. |
| `DIRTY_SESSION_EXIT_REJECTED` | Rule 3: Session end with dangling task or missing `next_action` | Complete or pause active task, and provide a concrete `next_action`. |
| `CYCLIC_DEPENDENCY` | Rule 6: Circular dependency on DAG | Remove recursive parent/child link. |
| `REFLECTION_REQUIRED` | Rule 7: Completed task threshold exceeded | Invoke `workflow_reflect` to summarize progress and review debt. |
| `ACTIVE_SESSION_CONFLICT`| Another active session holds the project lease | Wait for session to close, send heartbeat, or allow 300s timeout. |

---

## 8. Storage Architecture & Project Isolation

All data is sandboxed strictly under your configured `--projects-root`:

```
~/.taskmanager/projects/
├── compiler/
│   ├── memory.db                    # SQLite 3: sessions, observations (FTS5), blockers, audit
│   ├── taskwarrior/
│   │   ├── taskrc                   # Hermetic Taskwarrior configuration
│   │   └── data/                    # Taskwarrior persistence files
│   └── locks/
│       └── session.lock             # Cross-process flocking
└── webapp/
    ├── memory.db
    ├── taskwarrior/
    └── locks/
```

* Under no circumstances can an agent token issued for project `compiler` read or modify `webapp`.
* Path traversal attempts (such as `../`) are rejected immediately with `PROJECT_MISMATCH`.

---

## 9. Running Tests

To verify the installation and run the full test suite (including the real child-process stdio MCP integration test):

```bash
source .venv/bin/activate
pytest -v
```
