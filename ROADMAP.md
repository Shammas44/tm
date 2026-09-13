# Implementation Roadmap: TaskManager MCP Server

This document outlines the sequential, vertically integrated implementation slices for the **TaskManager MCP Server**.

> [!NOTE]
> The JWT Auth MCP Server is decoupled into a separate project to be implemented subsequently. For local development and testing of the TaskManager MCP, a self-contained test-key utility and configurable verification module will be used to mint and validate fixture tokens.

---

## Phase Overview

```
Phase 1: Environment & Project Scaffolding
  │
  ▼
Phase 2: Storage & Persistence Foundations
  ├── Taskwarrior Sandboxed Subprocess Bridge (UUIDv4 & UDAs)
  └── SQLite Memory Store (Sessions, FTS5 Observations, Blockers, Audit)
  │
  ▼
Phase 3: Domain Models & Lifecycle State Machines
  ├── Task State Machine (PENDING -> READY -> ACTIVE -> COMPLETED / BLOCKED)
  └── Session State Machine & Zombie Recovery
  │
  ▼
Phase 4: Workflow Invariant Enforcement Engine (Rules 1-7)
  ├── Session Gates & Evidence Verification
  ├── Dependency DAG & Blocker Rules
  └── Reflection Threshold Gate & Urgent Scheduler
  │
  ▼
Phase 5: Security & Verification Middleware
  ├── JWT Signature & Claims Validation (Ed25519 / RS256 / HS256)
  └── Project Sandboxing & RBAC Gate
  │
  ▼
Phase 6: Stdio MCP Server & Tool Registry
  ├── Stdio JSON-RPC Transport & Subprocess Handshake
  ├── Session & Task Execution Tools
  └── Observation, Blocker & Reflection Tools
  │
  ▼
Phase 7: End-to-End Integration & Agent Verification
  ├── Subprocess Stdio Integration Test Suite
  └── Host Configurations (Claude Desktop / Cursor / Custom Agent)
```

---

## Phase 1: Environment & Project Scaffolding

### Slice 1.1: Project Scaffolding & Toolchain Configuration
* **Objective:** Establish the Python 3.12+ project structure using `uv`, configure `pyproject.toml`, linting (`ruff`), and async testing (`pytest`, `pytest-asyncio`).
* **Deliverables:**
  * Root `pyproject.toml` for `taskmanager-mcp`.
  * Dependencies: `mcp`, `pydantic>=2.7`, `aiosqlite`, `pyjwt[crypto]`, `cryptography`, `structlog`, `pytest`, `pytest-asyncio`.
  * Test suite setup and baseline test execution.
* **Validation Criteria:** `uv sync` installs cleanly; `pytest` runs and passes basic health check.

### Slice 1.2: Shared Error Envelopes & Diagnostics
* **Objective:** Define structured domain exceptions, standard MCP JSON-RPC 2.0 error payloads, and a safe stderr logger.
* **Deliverables:**
  * Exception hierarchy (`TaskManagerError`, `InvariantViolationError`, `AuthenticationError`, `LockTimeoutError`).
  * MCP error serializer formatting structured error details (`code`, `message`, `data.error_code`, `data.remedy`, `data.retryable`).
  * `logger.py`: JSON structured logger strictly outputting to `sys.stderr` to prevent corrupting stdout.
* **Validation Criteria:** Unit tests verifying error envelope serialization conforms to MCP JSON-RPC 2.0.

---

## Phase 2: Storage & Persistence Foundations

### Slice 2.1: Hermetic Taskwarrior Subprocess Bridge
* **Objective:** Build an asynchronous Python wrapper around Taskwarrior isolating configuration per project.
* **Deliverables:**
  * Project-sandboxed `taskrc` and `data.location` initialization.
  * Custom agent UDAs (`uuid`, `component`, `confidence`, `estimate_minutes`, `owner`, `evidence`).
  * Cross-process file locking via `fcntl.flock` on `session.lock` with timeout.
  * Asynchronous subprocess executor (`task rc:<path> export`, `task modify`).
  * Strict mapping: all external interactions use immutable **UUIDv4**, never transient integer IDs.
* **Validation Criteria:** Unit tests creating, reading, and updating tasks across multiple isolated test project directories.

### Slice 2.2: SQLite Memory & Knowledge Store
* **Objective:** Implement project SQLite database (`memory.db`) with full-text search for durable knowledge.
* **Deliverables:**
  * SQLite connection manager with WAL mode enabled.
  * Tables: `sessions`, `observations`, `blockers`, `audit_log`, `reflections`.
  * FTS5 virtual table `observations_fts` with BM25 ranking and synchronization triggers.
* **Validation Criteria:** Observations inserted are immediately queryable via full-text search; BM25 relevance sorting functions.

---

## Phase 3: Domain Models & Lifecycle State Machines

### Slice 3.1: Task & Session Domain Models
* **Objective:** Create typed Pydantic v2 schemas and lifecycle state transitions.
* **Deliverables:**
  * Models: `Task`, `TaskAnnotation`, `Session`, `Observation`, `Blocker`, `Reflection`.
  * Task state machine (`PENDING`, `READY`, `ACTIVE`, `BLOCKED`, `COMPLETED`, `DELETED`).
  * Session state machine (`ACTIVE`, `SUSPENDED`, `CLOSED`, `ZOMBIE`).
  * Zombie session detection algorithm based on configurable heartbeat staleness.
* **Validation Criteria:** State transitions reject invalid paths (e.g. `PENDING` -> `COMPLETED` without `ACTIVE`).

---

## Phase 4: Workflow Invariant Enforcement Engine

### Slice 4.1: Session & Completion Invariants (Rules 1, 2, 3, 7)
* **Objective:** Implement pre- and post-condition checks governing session execution.
* **Deliverables:**
  * **Rule 1:** Prevent task modification or annotation unless the task is `ACTIVE` in the current session.
  * **Rule 2:** Require completion summary (>= 20 chars) and concrete verification evidence (>= 10 chars).
  * **Rule 3:** Reject `session_end` if an active task remains uncompleted/unpaused, or if unfinished work lacks `next_action`.
  * **Rule 7:** Enforce reflection threshold (trigger `REFLECTION_REQUIRED` after $N$ completed tasks).
* **Validation Criteria:** Unit tests verifying each rule violation returns the exact expected error code.

### Slice 4.2: Dependency DAG & Blocker Invariants (Rules 4, 5, 6)
* **Objective:** Enforce dependency graph integrity and blocker status synchronization.
* **Deliverables:**
  * **Rule 4:** Blocker registration auto-transitions target task to `BLOCKED`.
  * **Rule 5:** Knowledge taxonomy enforcement (durable discoveries vs transient task notes).
  * **Rule 6:** Dependency cycle detection (DFS on DAG) and prerequisite completion checks.
* **Validation Criteria:** Circular dependency creation fails with `CYCLIC_DEPENDENCY`; blocked tasks cannot be started.

### Slice 4.3: Priority Task Scheduler
* **Objective:** Implement the `task_get_next` selection algorithm.
* **Deliverables:**
  * Filter candidate tasks with all dependencies satisfied and no active blockers.
  * Sort by Taskwarrior urgency metrics (priority, age, tags, component).
  * Gracefully handle empty queues.
* **Validation Criteria:** Scheduler reliably selects the highest-urgency executable task.

---

## Phase 5: Security & Verification Middleware

### Slice 5.1: Embedded JWT Validator & Project Sandbox Guard
* **Objective:** Validate incoming JWT tokens and enforce strict project path confinement.
* **Deliverables:**
  * `jwt_validator.py`: Verifies Ed25519/RS256/HS256 tokens using public key or secret.
  * Claims validation: `sub`, `aud == "taskmanager-mcp"`, `project`, `role`, `permissions`, `exp`.
  * Test fixture helper: Generates signed test tokens for unit and integration testing.
  * Project directory guard: Verifies target project exists within `projects_root` and blocks directory traversal (`../`).
  * RBAC permission check per operation.
* **Validation Criteria:** Unit tests verify rejection of expired tokens, invalid signatures, wrong audience, and unauthorized projects.

---

## Phase 6: Stdio MCP Server & Tool Registry

### Slice 6.1: Stdio Transport Loop & Session Tools
* **Objective:** Implement MCP stdio JSON-RPC server with session handshake and lifecycle tools.
* **Deliverables:**
  * Stdio communication loop using MCP SDK.
  * Subprocess environment token extraction (`TASKMANAGER_AUTH_TOKEN`).
  * Tools: `session_begin`, `session_heartbeat`, `session_end`.
  * Automated audit trail logging.
* **Validation Criteria:** MCP client handshake succeeds; session opens and closes cleanly.

### Slice 6.2: Task Execution Tools
* **Objective:** Register all task-related MCP tools.
* **Deliverables:**
  * Tools: `task_get_next`, `task_start`, `task_create`, `task_create_subtask`, `task_complete`, `task_annotate`, `task_get`, `task_list`.
* **Validation Criteria:** End-to-end task lifecycle executes via MCP tool calls.

### Slice 6.3: Knowledge, Blocker & Reflection Tools
* **Objective:** Register observation, blocker, and reflection tools.
* **Deliverables:**
  * Tools: `observation_record`, `observation_search`, `blocker_create`, `blocker_resolve`, `workflow_reflect`.
* **Validation Criteria:** Observations are recorded and retrieved via BM25 full-text search; blockers transition task states.

---

## Phase 7: End-to-End Integration & Agent Verification

### Slice 7.1: Subprocess Stdio Integration Test Suite
* **Objective:** Run end-to-end test suite communicating over actual subprocess stdin/stdout pipes.
* **Deliverables:**
  * Integration tests simulating an agent session: spawn server -> `session_begin` -> create task -> start task -> complete with evidence -> record observation -> end session.
  * Context reset recovery test: second session restores context and `next_action`.
* **Validation Criteria:** Full subprocess lifecycle completes cleanly without timeouts or deadlocks.

### Slice 7.2: Agent Host Configurations & Quickstart
* **Objective:** Deliver configuration templates for Claude Desktop and Cursor.
* **Deliverables:**
  * `claude_desktop_config.json` configuration snippet.
  * Project bootstrap script to initialize a new project workspace.
  * Quickstart usage documentation.
* **Validation Criteria:** Claude Desktop starts TaskManager MCP server successfully over stdio.
