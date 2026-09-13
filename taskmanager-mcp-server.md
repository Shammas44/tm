# TaskManager MCP Server: Technical Specification (v2.0)
## High-Assurance Persistent Execution Memory & Workflow Governance for Autonomous Agents

---

## 1. Executive Summary & System Mission

The **TaskManager MCP Server** is a specialized Model Context Protocol (MCP) server operating exclusively over **standard input/output (`stdio`) transport**. It provides persistent, structured execution memory, workflow governance, and authoritative execution state for autonomous coding agents (such as Claude, Devin, or custom LLM harnesses).

### 1.1 Core Problems Solved
* **Context Reset Amnesia:** LLM agent sessions inevitably reset due to context window limits, compaction, or crashes. The TaskManager MCP preserves complete task graphs, uncommitted discoveries, blocked states, and historical rationale externally.
* **Ungoverned Execution Loops:** Unconstrained agents tend to jump directly into code edits without planning, skip verification, fail to document discoveries, and leave projects in half-finished, unrecoverable states. The TaskManager MCP acts as an invariant-enforcing state machine.
* **Direct Taskwarrior Exposure Pitfalls:** Exposing raw Taskwarrior CLI commands to LLMs leads to command injection, malformed filters, integer ID race conditions, and lack of domain semantics. This server wraps Taskwarrior behind a strict, semantic domain model.
* **Project Sandboxing & Agent Multi-Tenancy:** Prevents cross-project contamination and state corruption across multiple concurrent agents or subagents.

### 1.2 Operating Principles
1. **Authoritative Execution State:** The agent does not track state in scratchpads; the MCP server is the single source of truth.
2. **Deterministic Invariant Enforcement:** Transitions between work states require cryptographic and structural proof (e.g., active task required for edits, evidence required for completion).
3. **Stdio-Native Architecture:** Designed from the ground up for subprocess lifecycle management, standard Unix file descriptors, and zero-network overhead.
4. **Stable Identity Guarantees:** Strict reliance on immutable UUIDv4 identifiers rather than volatile Taskwarrior integer IDs.

---

## 2. System Architecture

```
                                  Agent Host Process
                         (Claude Desktop / Cursor / Custom Agent)
                                         │
                   ┌─────────────────────┴─────────────────────┐
                   │ Spawns subprocess with TASKMANAGER_AUTH_TOKEN
                   │ Communicates via newline-delimited JSON-RPC 2.0
                   ▼                                           ▼
             Standard Input (stdin)                  Standard Output (stdout)
                   │                                           ▲
┌──────────────────┼───────────────────────────────────────────┼──────────────────┐
│ TaskManager MCP  ▼                                           │                  │
│ Server Process   ┌───────────────────────────────────────────┴┐                 │
│                  │           Stdio JSON-RPC Transport         │                 │
│                  └─────────────────────┬──────────────────────┘                 │
│                                        │                                        │
│                                        ▼                                        │
│                  ┌───────────────────────────────────────────┐                  │
│                  │  Security Interceptor & Auth Middleware   │                  │
│                  │  (JWT Verification, RBAC, Project Guard)   │                  │
│                  └─────────────────────┬─────────────────────┘                  │
│                                        │                                        │
│                                        ▼                                        │
│                  ┌───────────────────────────────────────────┐                  │
│                  │    Workflow Invariant Enforcement Engine   │                  │
│                  │  (Session Gate, Dependency Graph, Rules)  │                  │
│                  └───────┬───────────────────────────┬───────┘                  │
│                          │                           │                          │
│                          ▼                           ▼                          │
│               ┌────────────────────┐      ┌────────────────────┐                │
│               │ Task Management    │      │ Session, Knowledge │                │
│               │ Domain Service     │      │ & Audit Service    │                │
│               └──────────┬─────────┘      └──────────┬─────────┘                │
└──────────────────────────┼───────────────────────────┼──────────────────────────┘
                           │                           │
                           ▼                           ▼
                 ┌───────────────────┐       ┌────────────────────┐
                 │ Taskwarrior CLI   │       │ SQLite 3 Database  │
                 │ Bridge (Subprocess│       │ • sessions         │
                 │ & TaskChampion)   │       │ • observations(FTS)│
                 │                   │       │ • blockers         │
                 │ • taskrc sandbox  │       │ • audit_trail      │
                 │ • data.location   │       │ • checkpoints      │
                 └───────────────────┘       └────────────────────┘
```

---

## 3. Stdio Transport & Security Architecture

### 3.1 Stdio Transport Constraints
In an MCP architecture running over `stdio` transport:
* The communication channel is local standard input (`stdin`) and standard output (`stdout`). Standard error (`stderr`) is strictly reserved for process diagnostics and human-readable logging.
* HTTP headers (such as `Authorization: Bearer <token>`) **do not exist**.
* The server process is typically spawned as a direct child process of the agent runner.

### 3.2 Dual Authentication Pattern for Stdio
To achieve high assurance without violating MCP transport boundaries, the server implements a two-tier authentication strategy:

```
Tier 1: Process Spawn Handshake (Environment Configuration)
  Agent Host sets TASKMANAGER_AUTH_TOKEN in subprocess env.
  Server verifies token at startup during MCP initialize.
  If invalid/missing/expired -> Server terminates with exit code 78 (EX_CONFIG) or sends MCP InitializeError.

Tier 2: Protocol Session Handshake (Explicit Tool Verification)
  Agent invokes `session_begin(auth_token=...)` tool.
  If auth_token omitted, server defaults to Tier 1 environment token.
  Server validates project binding, subject claim, role, and permissions.
  Establishes in-memory SessionSecurityContext bound to the stdio channel.
```

### 3.3 JWT Claims Specification
The server enforces the following standard and custom claims:

```json
{
  "iss": "mcp-jwt-auth-manager",
  "sub": "agent-claude-engineer-01",
  "aud": "taskmanager-mcp",
  "project": "compiler-core",
  "role": "worker",
  "permissions": [
    "session:start",
    "session:end",
    "task:read",
    "task:write",
    "obs:read",
    "obs:write",
    "blocker:manage"
  ],
  "session_max_duration_min": 240,
  "iat": 1773489600,
  "nbf": 1773489600,
  "exp": 1773518400,
  "jti": "d3b07384-d113-4c4e-b5c6-6a7c9f801f92"
}
```

| Claim | Type | Required | Description |
|---|---|---|---|
| `iss` | string | Yes | Must match configured trusted issuer (e.g. `mcp-jwt-auth-manager`). |
| `sub` | string | Yes | Unique identifier of the agent invoking the server. |
| `aud` | string | Yes | Must equal `taskmanager-mcp`. Prevents token misuse across different MCPs. |
| `project` | string | Yes | Project namespace slug. Enforces filesystem and database isolation. |
| `role` | string | Yes | One of: `admin`, `worker`, `reviewer`, `observer`. |
| `permissions`| array[string] | Yes | Specific capability strings (wildcards allowed, e.g. `task:*`). |
| `exp` | integer | Yes | Unix epoch expiration. Server rejects expired tokens with 0 clock tolerance. |
| `jti` | string | Yes | Unique token UUID used for revocation checks. |

### 3.4 Role-Based Access Control (RBAC) Matrix

| Permission | Description | Observer | Worker | Reviewer | Admin |
|---|---|:---:|:---:|:---:|:---:|
| `session:start` | Open session & restore context | ❌ | ✅ | ✅ | ✅ |
| `session:end` | Finalize session & invariants | ❌ | ✅ | ✅ | ✅ |
| `task:read` | Read tasks, dependencies, next | ✅ | ✅ | ✅ | ✅ |
| `task:write` | Create tasks, annotate, update | ❌ | ✅ | ✅ | ✅ |
| `task:complete`| Complete task with evidence | ❌ | ✅ | ❌ | ✅ |
| `task:approve` | Review & approve completed work| ❌ | ❌ | ✅ | ✅ |
| `task:admin` | Delete, purge, reparent tasks | ❌ | ❌ | ❌ | ✅ |
| `obs:read` | Search and read observations | ✅ | ✅ | ✅ | ✅ |
| `obs:write` | Record observations | ❌ | ✅ | ✅ | ✅ |
| `blocker:manage`| Create and resolve blockers | ❌ | ✅ | ✅ | ✅ |
| `audit:read` | Read security & execution logs | ❌ | ❌ | ✅ | ✅ |

### 3.5 Cryptographic Verification Pipeline
1. **Public Key Ingestion:** The server loads the public signing key (Ed25519 or RSA-2048/4096) from the path specified by `TASKMANAGER_PUBLIC_KEY_PATH` or the shared keystore directory (`~/.mcp-auth/keys/`).
2. **Signature & Expiration Check:** Validates signature using `PyJWT` with `cryptography`.
3. **Audience Check:** Confirms `aud == "taskmanager-mcp"`.
4. **Revocation Check:** Inspects local revocation SQLite database or shared CRL file for revoked `jti`.
5. **Project Confinement:** Ensures the requested project root directory exists within configured `PROJECTS_BASE_DIR`. Prevents path traversal (`../`).

---

## 4. Project Isolation & Storage Hierarchy

Each project is physically and logically partitioned. Under no circumstances may a server process operating under a token for `project-alpha` read or write to `project-beta`.

### 4.1 Filesystem Layout
```
/var/lib/taskmanager/projects/
├── compiler-core/
│   ├── project.yaml                 # Project-specific settings & invariant thresholds
│   ├── taskwarrior/
│   │   ├── taskrc                   # Isolated Taskwarrior configuration
│   │   └── data/                    # Taskwarrior storage (tasks.data / taskchampion.sqlite3)
│   │       ├── completed.data
│   │       ├── pending.data
│   │       └── undo.data
│   ├── memory.db                    # SQLite 3: sessions, observations, blockers, audit
│   └── locks/                       # Session and write locks
│       └── session.lock
└── kernel-driver/
    ├── project.yaml
    ├── taskwarrior/
    │   ├── taskrc
    │   └── data/
    ├── memory.db
    └── locks/
```

### 4.2 Sandboxed Taskwarrior Configuration (`taskrc`)
Every project generates a hermetic `taskrc` overriding all user-level defaults:
```ini
# Auto-generated by TaskManager MCP Server. DO NOT EDIT DIRECTLY.
data.location=/var/lib/taskmanager/projects/compiler-core/taskwarrior/data
locking=1
verbose=nothing
confirmation=no
hooks=off

# User-Defined Attributes (UDAs) for Autonomous Agent Semantics
uda.uuid.type=string
uda.uuid.label=UUID

uda.component.type=string
uda.component.label=Component

uda.confidence.type=numeric
uda.confidence.label=Confidence

uda.estimate_minutes.type=numeric
uda.estimate_minutes.label=EstimateMinutes

uda.owner.type=string
uda.owner.label=OwnerAgent

uda.session_id.type=string
uda.session_id.label=SessionID

uda.evidence.type=string
uda.evidence.label=CompletionEvidence
```

### 4.3 SQLite Memory Database Schema (`memory.db`)

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;

-- Agent Execution Sessions
CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,          -- UUIDv4
    agent_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    start_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    end_time TIMESTAMP NULL,
    status TEXT NOT NULL CHECK(status IN ('ACTIVE', 'SUSPENDED', 'CLOSED', 'ZOMBIE')),
    active_task_uuid TEXT NULL,
    next_action TEXT NULL,
    files_modified_count INTEGER DEFAULT 0,
    tests_run_count INTEGER DEFAULT 0,
    commits_count INTEGER DEFAULT 0,
    tasks_completed_count INTEGER DEFAULT 0,
    observations_count INTEGER DEFAULT 0,
    last_heartbeat TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Durable Engineering Knowledge (Observations)
CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY,                  -- UUIDv4
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    author_agent TEXT NOT NULL,
    topic TEXT NOT NULL,
    content TEXT NOT NULL,
    confidence REAL NOT NULL CHECK(confidence >= 0.0 AND confidence <= 1.0),
    component TEXT NOT NULL,
    code_references JSON NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Full-Text Search Virtual Table for Observations
CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    id UNINDEXED,
    topic,
    content,
    component,
    content='observations',
    content_rowid='rowid'
);

-- Triggers for FTS Synchronization
CREATE TRIGGER IF NOT EXISTS trg_obs_insert AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts(rowid, id, topic, content, component)
    VALUES (new.rowid, new.id, new.topic, new.content, new.component);
END;

CREATE TRIGGER IF NOT EXISTS trg_obs_delete AFTER DELETE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, id, topic, content, component)
    VALUES ('delete', old.rowid, old.id, old.topic, old.content, old.component);
END;

CREATE TRIGGER IF NOT EXISTS trg_obs_update AFTER UPDATE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, id, topic, content, component)
    VALUES ('delete', old.rowid, old.id, old.topic, old.content, old.component);
    INSERT INTO observations_fts(rowid, id, topic, content, component)
    VALUES (new.rowid, new.id, new.topic, new.content, new.component);
END;

-- Explicit Blockers
CREATE TABLE IF NOT EXISTS blockers (
    id TEXT PRIMARY KEY,                  -- UUIDv4
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    task_uuid TEXT NOT NULL,              -- Target blocked task UUID
    blocking_task_uuid TEXT NULL,         -- Dependent task UUID if applicable
    reason TEXT NOT NULL,
    severity TEXT NOT NULL CHECK(severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    status TEXT NOT NULL CHECK(status IN ('ACTIVE', 'RESOLVED')),
    resolution_summary TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP NULL
);

-- Append-Only Audit Trail
CREATE TABLE IF NOT EXISTS audit_log (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    session_id TEXT NULL,
    agent_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    action TEXT NOT NULL,
    target_resource TEXT NULL,
    success INTEGER NOT NULL CHECK(success IN (0, 1)),
    error_code TEXT NULL,
    payload_summary JSON NULL,
    execution_duration_ms INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Workflow Checkpoints & Reflection Records
CREATE TABLE IF NOT EXISTS reflections (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    agent_id TEXT NOT NULL,
    completed_tasks_count INTEGER NOT NULL,
    summary TEXT NOT NULL,
    stale_tasks_identified JSON NOT NULL DEFAULT '[]',
    recommendations JSON NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sessions_active ON sessions(project_id, status);
CREATE INDEX IF NOT EXISTS idx_blockers_task ON blockers(task_uuid, status);
CREATE INDEX IF NOT EXISTS idx_audit_agent ON audit_log(agent_id, timestamp);
```

---

## 5. Domain Models & Lifecycle State Machines

### 5.1 Task State Machine
In Taskwarrior, tasks have raw integer IDs that recalculate dynamically whenever tasks are completed or purged. **The TaskManager MCP guarantees that agents exclusively receive and operate on immutable UUIDv4 strings.**

```
                     ┌──────────────┐
                     │   PENDING    │
                     └──────┬───────┘
                            │
               ┌────────────┴────────────┐
               ▼                         ▼
   Dependencies Unresolved      Dependencies Resolved
               │                         │
               │                         ▼
               │                  ┌──────────────┐
               │                  │    READY     │
               │                  └──────┬───────┘
               │                         │
               │                         ▼ `task_start`
               │                  ┌──────────────┐
               └─────────────────►│    ACTIVE    │
                     Blocker      └──────┬───────┘
                    Detected             │
                       ▲                 ├────────────────────────┐
                       │                 ▼                        ▼
                       │          ┌──────────────┐         ┌──────────────┐
                       └──────────┤   BLOCKED    │         │  COMPLETED   │
                         `create_ └──────────────┘         └──────────────┘
                         blocker`                          (`complete_task`
                                                           + mandatory evidence)
```

#### Task Domain Object Schema
```python
from pydantic import BaseModel, Field
from typing import List, Optional
from datetime import datetime


class TaskAnnotation(BaseModel):
    timestamp: datetime
    author: str
    summary: str
    evidence: Optional[str] = None
    confidence: float = Field(ge=0.0, le=1.0)


class Task(BaseModel):
    uuid: str = Field(description="Immutable UUIDv4")
    integer_id: Optional[int] = Field(description="Transient Taskwarrior ID for human readability")
    title: str
    description: str
    status: str = Field(description="PENDING, READY, ACTIVE, BLOCKED, COMPLETED, DELETED")
    priority: str = Field(description="H, M, L, or NONE")
    dependencies: List[str] = Field(default_factory=list, description="List of blocking Task UUIDs")
    tags: List[str] = Field(default_factory=list)
    component: str
    confidence: float = Field(ge=0.0, le=1.0)
    estimate_minutes: Optional[int] = None
    owner: Optional[str] = None
    annotations: List[TaskAnnotation] = Field(default_factory=list)
    created_at: datetime
    updated_at: Optional[datetime] = None
```

### 5.2 Session Lifecycle & Zombie Recovery
A session corresponds to a bounded working window for an agent on a specific project.
* **Invariant:** Only **one** session may be in `ACTIVE` status per agent per project at any given time.
* **Zombie Session Detection:** If an agent's process terminates abruptly, its session remains `ACTIVE` in SQLite. When an agent calls `session_begin()`, the server checks for existing `ACTIVE` sessions:
  1. If the last heartbeat occurred within `stale_session_threshold_seconds` (default: 300s), the call fails with `ACTIVE_SESSION_CONFLICT`.
  2. If the heartbeat is stale (>300s), the server transitions the previous session to `ZOMBIE`, preserves its `active_task_uuid`, logs an audit warning, and permits a clean session recovery.

---

## 6. Workflow Invariant Enforcement Rules

The core value of this MCP is that it strictly enforces engineering discipline. Every invariant is validated by Python middleware before delegating to Taskwarrior or SQLite.

### Rule 1: No Unregistered Progress
* **Invariant:** An agent cannot mark tasks complete, add completion evidence, or record task-linked discoveries unless the referenced task is currently marked `ACTIVE` by the calling session.
* **Failure Code:** `TASK_NOT_ACTIVE`

### Rule 2: Non-Negotiable Completion Evidence
* **Invariant:** Calling `task_complete(uuid=...)` fails unless provided with:
  1. A non-empty `summary` (minimum 20 characters explaining *what* was accomplished).
  2. Concrete `evidence` (e.g. test command output, git diff summary, verification command run).
  3. A confidence score between 0.0 and 1.0.
* **Failure Code:** `COMPLETION_EVIDENCE_MISSING`

### Rule 3: Clean Exit Invariant
* **Invariant:** Calling `session_end()` fails if:
  1. There is an `ACTIVE` task that was neither completed nor explicitly demoted to `READY`/`BLOCKED`.
  2. The agent has uncompleted tasks in progress and fails to provide a structured `next_action` string detailing the immediate next step for the next agent or session.
* **Failure Code:** `DIRTY_SESSION_EXIT_REJECTED`

### Rule 4: Explicit Blocker Reference
* **Invariant:** Every blocker registered via `blocker_create()` must reference a valid, non-completed Task UUID. The referenced task's status is automatically set to `BLOCKED`. A blocked task will never be returned by `task_get_next()`.
* **Failure Code:** `INVALID_BLOCKER_TARGET`

### Rule 5: Knowledge Separation
* **Invariant:** Durable architectural decisions and persistent codebase quirks must be recorded via `observation_record()`. Transient progress notes must be recorded via `task_annotate()`. The system rejects observations with trivial or transient content.

### Rule 6: Dependency DAG Enforcement
* **Invariant:** A task cannot be started (`task_start`) or selected as next (`task_get_next`) if any task listed in its `dependencies` array is in a state other than `COMPLETED`. Cyclic dependencies are rejected at task creation time via depth-first search on the dependency graph.
* **Failure Code:** `UNRESOLVED_DEPENDENCY_BLOCK`

### Rule 7: Reflection Gate
* **Invariant:** After every $N$ completed tasks (default: 5, configurable in `project.yaml`), `task_get_next()` will refuse to dispatch new tasks, returning error `REFLECTION_REQUIRED`. The agent is forced to invoke `workflow_reflect()` to review stale tasks, identify architectural debt, and plan subsequent milestones.
* **Failure Code:** `REFLECTION_REQUIRED`

---

## 7. MCP Tool Specifications

The server exposes 15 strictly typed tools over stdio JSON-RPC.

### 7.1 Session Management Tools

#### `session_begin`
Initiates an authenticated agent session, recovers prior context, and retrieves the active state.

* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "auth_token": {
        "type": "string",
        "description": "JWT token. Optional if TASKMANAGER_AUTH_TOKEN is supplied via environment."
      },
      "agent_id": {
        "type": "string",
        "description": "Unique agent identifier (must match JWT sub claim)."
      },
      "project": {
        "type": "string",
        "description": "Project identifier (must match JWT project claim)."
      }
    },
    "required": ["agent_id", "project"]
  }
  ```
* **Returns:**
  ```json
  {
    "session_id": "8fa21e29-65d1-4db8-b59a-526bcf4bf27b",
    "project": "compiler-core",
    "agent_id": "agent-claude-engineer-01",
    "active_task": {
      "uuid": "4bf77239-4a7b-4ee9-b4f0-bc37042a969f",
      "title": "Implement LR(1) parser table generator",
      "status": "ACTIVE"
    },
    "active_blockers": [],
    "recent_observations": [
      {
        "id": "e0b82f80-0a7f-4ca6-bbf4-6019eb9a6a84",
        "topic": "Grammar Ambiguity",
        "content": "Shift/reduce conflict on dangling else is resolved via precedence rule."
      }
    ],
    "previous_session_next_action": "Run benchmark suite after grammar table serialization."
  }
  ```

#### `session_heartbeat`
Refreshes the session's active timestamp to prevent zombie reaping during long-running builds/tests.
* **Parameters:** `{"session_id": {"type": "string"}}`
* **Returns:** `{"status": "OK", "timestamp": "2026-09-13T13:40:00Z"}`

#### `session_end`
Validates session invariants, records metrics, and safely closes the session.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "session_id": {"type": "string"},
      "next_action": {
        "type": "string",
        "description": "Mandatory next action if work remains pending."
      },
      "files_modified_count": {"type": "integer", "default": 0},
      "tests_run_count": {"type": "integer", "default": 0},
      "commits_count": {"type": "integer", "default": 0}
    },
    "required": ["session_id"]
  }
  ```
* **Returns:** `{"status": "CLOSED", "session_id": "...", "duration_minutes": 42}`

---

### 7.2 Task Execution & Scheduling Tools

#### `task_get_next`
Calculates and returns the single highest-priority, executable task with all dependencies fulfilled.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "component": {
        "type": "string",
        "description": "Optional component filter (e.g. 'lexer', 'codegen')"
      }
    }
  }
  ```
* **Selection Algorithm:**
  1. Filter tasks where `status == 'PENDING'` and all `dependencies` have `status == 'COMPLETED'`.
  2. Exclude tasks linked to active blockers in SQLite.
  3. Order by Taskwarrior urgency score (calculated from Priority, Due Date, Age, and Tags).
  4. Returns `null` if no executable tasks remain.

#### `task_start`
Transitions a task to `ACTIVE`. Automatically registers the task as the calling session's active task.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "task_uuid": {"type": "string"}
    },
    "required": ["task_uuid"]
  }
  ```
* **Invariant Checks:**
  * Rejects if another task is already `ACTIVE` in this session without being paused or completed.
  * Rejects if dependencies are not fulfilled.

#### `task_create`
Creates a new task with full metadata and dependency validation.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "title": {"type": "string", "minLength": 5},
      "description": {"type": "string"},
      "component": {"type": "string"},
      "priority": {"type": "string", "enum": ["H", "M", "L", "NONE"], "default": "M"},
      "dependencies": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Array of parent Task UUIDs that must complete first."
      },
      "estimate_minutes": {"type": "integer"},
      "confidence": {"type": "number", "minimum": 0.0, "maximum": 1.0, "default": 1.0},
      "tags": {"type": "array", "items": {"type": "string"}}
    },
    "required": ["title", "component"]
  }
  ```
* **Returns:** `{"task_uuid": "...", "status": "PENDING"}`

#### `task_create_subtask`
Convenience tool that creates a child task and automatically establishes a dependency link (`parent depends on child`).
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "parent_task_uuid": {"type": "string"},
      "title": {"type": "string"},
      "description": {"type": "string"},
      "estimate_minutes": {"type": "integer"}
    },
    "required": ["parent_task_uuid", "title"]
  }
  ```

#### `task_complete`
Marks an active task as completed. Enforces Rule 2.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "task_uuid": {"type": "string"},
      "summary": {
        "type": "string",
        "minLength": 20,
        "description": "Detailed explanation of changes made and resolution."
      },
      "evidence": {
        "type": "string",
        "minLength": 10,
        "description": "Concrete proof (test execution output, hash, command transcript)."
      },
      "confidence": {"type": "number", "minimum": 0.0, "maximum": 1.0}
    },
    "required": ["task_uuid", "summary", "evidence", "confidence"]
  }
  ```

#### `task_annotate`
Attaches structured observations and incremental notes to an existing task.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "task_uuid": {"type": "string"},
      "summary": {"type": "string"},
      "evidence": {"type": "string"},
      "confidence": {"type": "number", "minimum": 0.0, "maximum": 1.0}
    },
    "required": ["task_uuid", "summary"]
  }
  ```

#### `task_get`
Retrieves full task details including dependencies, dependents, and annotations.
* **Parameters:** `{"task_uuid": {"type": "string"}}`

#### `task_list`
Lists tasks with filters for status, component, priority, and tags.

---

### 7.3 Knowledge Management (Observations)

#### `observation_record`
Persists durable knowledge into the project SQLite store and updates the FTS5 index.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "topic": {"type": "string", "minLength": 3},
      "content": {"type": "string", "minLength": 15},
      "component": {"type": "string"},
      "confidence": {"type": "number", "minimum": 0.0, "maximum": 1.0},
      "code_references": {
        "type": "array",
        "items": {"type": "string"},
        "description": "File paths or symbols (e.g. 'src/parser/lexer.rs:120')"
      }
    },
    "required": ["topic", "content", "component", "confidence"]
  }
  ```
* **Returns:** `{"observation_id": "...", "status": "STORED"}`

#### `observation_search`
Full-text search over durable observations using SQLite FTS5 with BM25 ranking.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "query": {"type": "string"},
      "component": {"type": "string"},
      "min_confidence": {"type": "number", "default": 0.0},
      "limit": {"type": "integer", "default": 10}
    },
    "required": ["query"]
  }
  ```

---

### 7.4 Blocker Management

#### `blocker_create`
Registers a blocker, auto-transitions the affected task to `BLOCKED`, and records severity.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "task_uuid": {"type": "string"},
      "blocking_task_uuid": {"type": "string"},
      "reason": {"type": "string", "minLength": 10},
      "severity": {"type": "string", "enum": ["LOW", "MEDIUM", "HIGH", "CRITICAL"]}
    },
    "required": ["task_uuid", "reason", "severity"]
  }
  ```

#### `blocker_resolve`
Resolves an existing blocker and transitions the task back to `READY` if no other blockers exist.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "blocker_id": {"type": "string"},
      "resolution_summary": {"type": "string", "minLength": 10}
    },
    "required": ["blocker_id", "resolution_summary"]
  }
  ```

---

### 7.5 Workflow Governance & Reflection

#### `workflow_reflect`
Analyzes recent completed tasks, detects stale tasks, and generates recommendations. Satisfies Rule 7.
* **Parameters:**
  ```json
  {
    "type": "object",
    "properties": {
      "summary": {
        "type": "string",
        "description": "Agent analysis of architectural progress and debt."
      },
      "stale_tasks_identified": {
        "type": "array",
        "items": {"type": "string"}
      },
      "recommendations": {
        "type": "array",
        "items": {"type": "string"}
      }
    },
    "required": ["summary"]
  }
  ```
* **Returns:**
  ```json
  {
    "reflection_id": "...",
    "tasks_completed_since_last_reflection": 5,
    "reflection_gate_cleared": true
  }
  ```

---

## 8. Backend Integration: Taskwarrior Bridge

The server isolates all interactions with Taskwarrior behind an asynchronous Python wrapper (`TaskwarriorBridge`).

### 8.1 Execution Mechanics
* Invokes the Taskwarrior binary via `asyncio.create_subprocess_exec`.
* Forces explicit configuration via arguments:
  ```bash
  task rc:/var/lib/taskmanager/projects/{project}/taskwarrior/taskrc rc.confirmation=no export
  ```
* Ingestion of raw output uses strictly formatted JSON:
  ```bash
  task rc:<path> export status:pending
  ```
* Modifications pass UUIDs directly:
  ```bash
  task rc:<path> {uuid} modify status:completed evidence:"..."
  ```

### 8.2 File Locking & Multi-Process Safety
Taskwarrior 2.x and 3.x implement file-level locking. To prevent deadlocks when an agent spawns parallel tool queries:
1. `TaskwarriorBridge` encapsulates an `asyncio.Lock()` per project in the Python process.
2. File-system level locking is enforced using Python's `fcntl.flock(fd, fcntl.LOCK_EX)` on `locks/session.lock` with a 5000ms acquisition timeout before raising `STORAGE_LOCK_TIMEOUT`.

---

## 9. Standardized Error Handling

All tool errors adhere to the MCP JSON-RPC 2.0 error specification, augmented with structured application metadata.

### 9.1 Error Response Envelope
```json
{
  "code": -32000,
  "message": "Workflow Invariant Violation: COMPLETION_EVIDENCE_MISSING",
  "data": {
    "error_code": "COMPLETION_EVIDENCE_MISSING",
    "retryable": false,
    "task_uuid": "4bf77239-4a7b-4ee9-b4f0-bc37042a969f",
    "detail": "Field 'evidence' must contain at least 10 characters detailing verification output.",
    "remedy": "Run the corresponding test suite or build command, capture stdout/stderr, and provide it in the 'evidence' parameter."
  }
}
```

### 9.2 Error Code Registry

| Error Code | HTTP-Equivalent | Description |
|---|---|---|
| `UNAUTHENTICATED` | 401 | Missing, malformed, or expired JWT token. |
| `FORBIDDEN_PERMISSION`| 403 | Token lacks required permission for this tool. |
| `PROJECT_MISMATCH` | 403 | Token project claim does not match requested project. |
| `ACTIVE_SESSION_CONFLICT` | 409 | Another active session is currently leased to this project. |
| `TASK_NOT_ACTIVE` | 400 | Attempted to modify/complete a task that has not been started. |
| `COMPLETION_EVIDENCE_MISSING` | 422 | Complete task failed due to lack of verification evidence. |
| `DIRTY_SESSION_EXIT_REJECTED` | 422 | Attempted to end session with an active uncompleted task. |
| `REFLECTION_REQUIRED` | 428 | Reflection threshold exceeded; agent must call `workflow_reflect`. |
| `CYCLIC_DEPENDENCY` | 400 | Attempted to create a dependency graph with a cycle. |
| `STORAGE_LOCK_TIMEOUT` | 503 | Could not acquire lock on Taskwarrior database. |

---

## 10. Audit Logging & Telemetry

Every JSON-RPC call passing through the stdio transport is logged to the `audit_log` table in `memory.db` and echoed to `stderr` in structured JSON format:

```json
{
  "timestamp": "2026-09-13T13:42:01.104Z",
  "level": "INFO",
  "event": "TOOL_INVOCATION",
  "agent_id": "agent-claude-engineer-01",
  "project": "compiler-core",
  "tool": "task_complete",
  "target_uuid": "4bf77239-4a7b-4ee9-b4f0-bc37042a969f",
  "success": true,
  "duration_ms": 14
}
```

---

## 11. Configuration & Host Integration

### 11.1 Server Configuration (`taskmanager.yaml`)
```yaml
server:
  host_stdio: true
  log_level: "INFO"
  projects_root: "/var/lib/taskmanager/projects"

security:
  jwt_issuer: "mcp-jwt-auth-manager"
  jwt_audience: "taskmanager-mcp"
  public_key_path: "/etc/mcp/keys/public.pem"
  stale_session_threshold_seconds: 300

workflow:
  reflection_threshold_tasks: 5
  strict_evidence_validation: true
  min_evidence_length: 10
```

### 11.2 Claude Desktop Configuration Snippet (`claude_desktop_config.json`)
```json
{
  "mcpServers": {
    "taskmanager": {
      "command": "uv",
      "args": [
        "run",
        "--with", "taskmanager-mcp",
        "taskmanager-mcp",
        "--config", "/etc/mcp/taskmanager.yaml"
      ],
      "env": {
        "TASKMANAGER_AUTH_TOKEN": "eyJhbGciOiJFZERTQ...",
        "TASKMANAGER_PUBLIC_KEY_PATH": "/etc/mcp/keys/public.pem"
      }
    }
  }
}
```

---

## 12. Python Implementation Stack & File Structure

### 12.1 Core Dependencies
* **Python Runtime:** 3.12+
* **MCP Framework:** `mcp` (Official Anthropic Python MCP SDK)
* **Data Validation:** `pydantic` v2.7+
* **Database & ORM:** `aiosqlite`, `SQLAlchemy` 2.0+ (async Core/ORM)
* **Cryptography:** `pyjwt[crypto]`, `cryptography`
* **Logging & Observability:** `structlog`

### 12.2 Module Architecture
```
taskmanager_mcp/
├── __init__.py
├── __main__.py                      # Entry point for stdio execution
├── server.py                        # MCP Server lifecycle & tool registrations
├── config.py                        # Configuration parser & Pydantic models
├── security/
│   ├── jwt_validator.py            # Signature, claims, and revocation validation
│   └── context.py                   # Thread/task-local SecurityContext
├── domain/
│   ├── task.py                      # Task domain entity & Pydantic schemas
│   ├── session.py                   # Session entity & state machine
│   ├── observation.py               # Observation entity & FTS queries
│   └── blocker.py                   # Blocker entity
├── engine/
│   ├── invariants.py                # Rules 1-7 workflow validator
│   └── scheduler.py                 # Priority & DAG execution ordering
├── storage/
│   ├── taskwarrior_bridge.py        # Taskwarrior CLI subprocess wrapper
│   ├── sqlite_store.py              # SQLite sessions/observations repository
│   └── lock_manager.py              # Cross-process flocking
└── utils/
    └── logger.py                    # Stderr-only JSON structured logger
```
