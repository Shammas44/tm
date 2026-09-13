# Implementation Progress: TaskManager MCP Server

This document tracks implementation progress across all engineering slices defined in [ROADMAP.md](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/ROADMAP.md).

> [!NOTE]
> The JWT Auth MCP is deferred to a dedicated subsequent project. TaskManager MCP implementation includes embedded test token utilities for testing and development.

**Legend:**
- `[ ]` Not started
- `[/]` In progress
- `[x]` Completed & verified with tests

---

## Phase 1: Environment & Project Scaffolding
- [ ] **Slice 1.1: Project Scaffolding & Toolchain Configuration**
  - [ ] Initialize Python 3.12+ project with `uv`
  - [ ] Configure `pyproject.toml` with `mcp`, `pydantic>=2.7`, `aiosqlite`, `pyjwt[crypto]`, `cryptography`, `structlog`
  - [ ] Configure `ruff` (linting & formatting) and `pytest` with async support
- [ ] **Slice 1.2: Shared Error Envelopes & Diagnostics**
  - [ ] Implement domain exception hierarchy (`TaskManagerError`, `InvariantViolationError`, etc.)
  - [ ] Implement MCP JSON-RPC 2.0 error data payload serializer
  - [ ] Configure stderr-only structured logger (preserving stdout for stdio transport)

---

## Phase 2: Storage & Persistence Foundations
- [ ] **Slice 2.1: Hermetic Taskwarrior Subprocess Bridge**
  - [ ] Implement project-sandboxed `taskrc` and `data.location` initialization
  - [ ] Register agent UDAs (`uuid`, `component`, `confidence`, `estimate_minutes`, `evidence`)
  - [ ] Implement cross-process file locking (`fcntl.flock` on `session.lock`)
  - [ ] Implement asynchronous CLI wrapper (`task rc:<path> export`)
  - [ ] Enforce strict mapping to immutable **UUIDv4** across all public interfaces
- [ ] **Slice 2.2: SQLite Memory & Knowledge Store**
  - [ ] Initialize `memory.db` with WAL mode
  - [ ] Implement schema for `sessions`, `observations`, `blockers`, `audit_log`, `reflections`
  - [ ] Configure `observations_fts` (SQLite FTS5 virtual table with BM25 ranking)
  - [ ] Implement FTS synchronization triggers (insert, update, delete)

---

## Phase 3: Domain Models & Lifecycle State Machines
- [ ] **Slice 3.1: Task & Session Domain Models**
  - [ ] Define Pydantic models: `Task`, `TaskAnnotation`, `Session`, `Observation`, `Blocker`, `Reflection`
  - [ ] Implement task state machine (`PENDING`, `READY`, `ACTIVE`, `BLOCKED`, `COMPLETED`, `DELETED`)
  - [ ] Implement session state machine & zombie detection via stale heartbeats

---

## Phase 4: Workflow Invariant Enforcement Engine
- [ ] **Slice 4.1: Session & Completion Invariants (Rules 1, 2, 3, 7)**
  - [ ] Enforce **Rule 1**: Active task required before task modification/progress
  - [ ] Enforce **Rule 2**: Mandatory completion summary (>=20 chars) and evidence (>=10 chars)
  - [ ] Enforce **Rule 3**: Clean session exit (requires `next_action` if unfinished tasks exist)
  - [ ] Enforce **Rule 7**: Reflection gate threshold (triggers `REFLECTION_REQUIRED`)
- [ ] **Slice 4.2: Dependency DAG & Blocker Invariants (Rules 4, 5, 6)**
  - [ ] Enforce **Rule 4**: Blocker binds to task and sets status to `BLOCKED`
  - [ ] Enforce **Rule 5**: Knowledge taxonomy (durable observations vs task annotations)
  - [ ] Enforce **Rule 6**: Dependency cycle detection (DFS on DAG) & prerequisite completion check
- [ ] **Slice 4.3: Priority Task Scheduler**
  - [ ] Implement `task_get_next` calculation based on urgency, dependencies, blockers, and reflection state

---

## Phase 5: Security & Verification Middleware
- [ ] **Slice 5.1: Embedded JWT Validator & Project Sandbox Guard**
  - [ ] Implement JWT signature verification (Ed25519, RS256, HS256)
  - [ ] Implement claims validation (`sub`, `aud == "taskmanager-mcp"`, `project`, `role`, `permissions`, `exp`)
  - [ ] Implement test fixture token generator for unit & integration tests
  - [ ] Implement project directory path confinement and traversal guard
  - [ ] Implement RBAC permission check per operation

---

## Phase 6: Stdio MCP Server & Tool Registry
- [ ] **Slice 6.1: Stdio Transport Loop & Session Tools**
  - [ ] Implement stdio JSON-RPC server loop using MCP SDK
  - [ ] Implement subprocess environment token extraction (`TASKMANAGER_AUTH_TOKEN`)
  - [ ] Register `session_begin`, `session_heartbeat`, `session_end`
  - [ ] Implement append-only audit trail logging
- [ ] **Slice 6.2: Task Execution Tools**
  - [ ] Register `task_get_next`, `task_start`, `task_create`, `task_create_subtask`
  - [ ] Register `task_complete`, `task_annotate`, `task_get`, `task_list`
- [ ] **Slice 6.3: Knowledge, Blocker & Reflection Tools**
  - [ ] Register `observation_record` and `observation_search` (FTS5 BM25)
  - [ ] Register `blocker_create` and `blocker_resolve`
  - [ ] Register `workflow_reflect`

---

## Phase 7: End-to-End Integration & Agent Verification
- [ ] **Slice 7.1: Subprocess Stdio Integration Test Suite**
  - [ ] Test end-to-end token minting -> stdio process spawn -> session initialization
  - [ ] Test complete workflow loop (task creation -> active -> evidence -> completion -> exit)
  - [ ] Test context reset recovery (second session recovers state & next action)
  - [ ] Test invariant violation rejections over stdio transport
- [ ] **Slice 7.2: Agent Host Configurations & Quickstart**
  - [ ] Provide ready-to-use Claude Desktop configuration
  - [ ] Provide ready-to-use Cursor configuration
  - [ ] Create project bootstrap and smoke-test script
