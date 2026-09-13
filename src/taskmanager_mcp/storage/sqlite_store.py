"""SQLite persistence engine for sessions, observations, blockers, and audit logs.

Provides ACID transaction guarantees and FTS5 full-text search with BM25 ranking.
"""

import json
from pathlib import Path
from typing import Any

import aiosqlite

from taskmanager_mcp.errors import ResourceNotFoundError
from taskmanager_mcp.utils.logger import get_logger

logger = get_logger("sqlite_store")

SCHEMA_SQL = """
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
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

CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY,
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

CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    id UNINDEXED,
    topic,
    content,
    component,
    content='observations',
    content_rowid='rowid'
);

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

CREATE TABLE IF NOT EXISTS blockers (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    task_uuid TEXT NOT NULL,
    blocking_task_uuid TEXT NULL,
    reason TEXT NOT NULL,
    severity TEXT NOT NULL CHECK(severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    status TEXT NOT NULL CHECK(status IN ('ACTIVE', 'RESOLVED')),
    resolution_summary TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP NULL
);

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
CREATE INDEX IF NOT EXISTS idx_audit_project ON audit_log(project_id, timestamp);
"""


class SQLiteMemoryStore:
    """Async SQLite persistence store for project execution memory."""

    def __init__(self, db_path: Path):
        self.db_path = db_path.resolve()
        self.db_path.parent.mkdir(parents=True, exist_ok=True)

    async def initialize(self) -> None:
        """Executes database schema migrations and PRAGMA settings."""
        async with aiosqlite.connect(self.db_path) as db:
            await db.executescript(SCHEMA_SQL)
            await db.commit()
        logger.info("initialized_sqlite_store", db_path=str(self.db_path))

    # --- Session Management ---

    async def create_session(
        self,
        session_id: str,
        agent_id: str,
        project_id: str,
    ) -> dict[str, Any]:
        """Creates an ACTIVE session."""
        query = """
            INSERT INTO sessions (session_id, agent_id, project_id, status, last_heartbeat)
            VALUES (?, ?, ?, 'ACTIVE', CURRENT_TIMESTAMP)
        """
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(query, (session_id, agent_id, project_id))
            await db.commit()
        return await self.get_session(session_id)  # type: ignore

    async def get_session(self, session_id: str) -> dict[str, Any] | None:
        """Retrieves a session by ID."""
        query = "SELECT * FROM sessions WHERE session_id = ?"
        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(query, (session_id,)) as cursor:
                row = await cursor.fetchone()
                return dict(row) if row else None

    async def get_active_session(self, project_id: str) -> dict[str, Any] | None:
        """Retrieves currently ACTIVE session for the given project."""
        query = "SELECT * FROM sessions WHERE project_id = ? AND status = 'ACTIVE' LIMIT 1"
        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(query, (project_id,)) as cursor:
                row = await cursor.fetchone()
                return dict(row) if row else None

    async def heartbeat_session(self, session_id: str) -> bool:
        """Updates last_heartbeat timestamp to CURRENT_TIMESTAMP."""
        query = "UPDATE sessions SET last_heartbeat = CURRENT_TIMESTAMP WHERE session_id = ?"
        async with aiosqlite.connect(self.db_path) as db:
            cursor = await db.execute(query, (session_id,))
            await db.commit()
            return cursor.rowcount > 0

    async def update_session(self, session_id: str, **fields: Any) -> dict[str, Any]:
        """Updates arbitrary session fields."""
        if not fields:
            sess = await self.get_session(session_id)
            if not sess:
                raise ResourceNotFoundError("Session", session_id)
            return sess

        set_clause = ", ".join(f"{k} = ?" for k in fields.keys())
        values = list(fields.values()) + [session_id]
        query = f"UPDATE sessions SET {set_clause} WHERE session_id = ?"

        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(query, values)
            await db.commit()

        updated = await self.get_session(session_id)
        if not updated:
            raise ResourceNotFoundError("Session", session_id)
        return updated

    async def close_session(
        self,
        session_id: str,
        next_action: str | None = None,
        files_modified_count: int = 0,
        tests_run_count: int = 0,
        commits_count: int = 0,
    ) -> dict[str, Any]:
        """Marks a session as CLOSED with execution metrics."""
        query = """
            UPDATE sessions
            SET status = 'CLOSED',
                end_time = CURRENT_TIMESTAMP,
                active_task_uuid = NULL,
                next_action = ?,
                files_modified_count = files_modified_count + ?,
                tests_run_count = tests_run_count + ?,
                commits_count = commits_count + ?
            WHERE session_id = ?
        """
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(
                query,
                (next_action, files_modified_count, tests_run_count, commits_count, session_id),
            )
            await db.commit()

        closed = await self.get_session(session_id)
        if not closed:
            raise ResourceNotFoundError("Session", session_id)
        return closed

    # --- Knowledge Observations & FTS5 ---

    async def record_observation(
        self,
        observation_id: str,
        project_id: str,
        session_id: str,
        author_agent: str,
        topic: str,
        content: str,
        confidence: float,
        component: str,
        code_references: list[str] | None = None,
    ) -> str:
        """Persists a durable observation and synchronizes FTS5 index."""
        query = """
            INSERT INTO observations
            (id, project_id, session_id, author_agent, topic, content, confidence, component, code_references)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """
        refs_json = json.dumps(code_references or [])
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(
                query,
                (
                    observation_id,
                    project_id,
                    session_id,
                    author_agent,
                    topic,
                    content,
                    confidence,
                    component,
                    refs_json,
                ),
            )
            await db.execute(
                "UPDATE sessions SET observations_count = observations_count + 1 WHERE session_id = ?",
                (session_id,),
            )
            await db.commit()
        return observation_id

    async def search_observations(
        self,
        project_id: str,
        query: str,
        component: str | None = None,
        min_confidence: float = 0.0,
        limit: int = 10,
    ) -> list[dict[str, Any]]:
        """Performs full-text search with BM25 ranking across observations."""
        # Sanitize query for FTS5: escape quotes and wrap terms
        sanitized_terms = [f'"{term.replace('"', '""')}"' for term in query.split() if term]
        if not sanitized_terms:
            return []
        fts_match_expr = " AND ".join(sanitized_terms)

        sql = """
            SELECT o.id, o.project_id, o.session_id, o.author_agent, o.topic, o.content,
                   o.confidence, o.component, o.code_references, o.created_at,
                   bm25(observations_fts) as rank
            FROM observations o
            JOIN observations_fts fts ON o.rowid = fts.rowid
            WHERE observations_fts MATCH ?
              AND o.project_id = ?
              AND o.confidence >= ?
        """
        params: list[Any] = [fts_match_expr, project_id, min_confidence]
        if component:
            sql += " AND o.component = ?"
            params.append(component)

        sql += " ORDER BY rank ASC LIMIT ?"
        params.append(limit)

        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(sql, params) as cursor:
                rows = await cursor.fetchall()
                results = []
                for row in rows:
                    item = dict(row)
                    item["code_references"] = json.loads(item["code_references"])
                    results.append(item)
                return results

    # --- Blockers Management ---

    async def create_blocker(
        self,
        blocker_id: str,
        project_id: str,
        session_id: str,
        task_uuid: str,
        reason: str,
        severity: str,
        blocking_task_uuid: str | None = None,
    ) -> str:
        """Registers a new active blocker."""
        query = """
            INSERT INTO blockers
            (id, project_id, session_id, task_uuid, blocking_task_uuid, reason, severity, status)
            VALUES (?, ?, ?, ?, ?, ?, ?, 'ACTIVE')
        """
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(
                query,
                (
                    blocker_id,
                    project_id,
                    session_id,
                    task_uuid,
                    blocking_task_uuid,
                    reason,
                    severity,
                ),
            )
            await db.commit()
        return blocker_id

    async def get_blocker(self, blocker_id: str) -> dict[str, Any] | None:
        """Retrieves blocker by ID."""
        query = "SELECT * FROM blockers WHERE id = ?"
        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(query, (blocker_id,)) as cursor:
                row = await cursor.fetchone()
                return dict(row) if row else None

    async def list_active_blockers(
        self,
        project_id: str,
        task_uuid: str | None = None,
    ) -> list[dict[str, Any]]:
        """Lists currently ACTIVE blockers, optionally filtered by task UUID."""
        query = "SELECT * FROM blockers WHERE project_id = ? AND status = 'ACTIVE'"
        params = [project_id]
        if task_uuid:
            query += " AND task_uuid = ?"
            params.append(task_uuid)
        query += " ORDER BY created_at DESC"

        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(query, params) as cursor:
                rows = await cursor.fetchall()
                return [dict(row) for row in rows]

    async def resolve_blocker(self, blocker_id: str, resolution_summary: str) -> dict[str, Any]:
        """Resolves an active blocker."""
        query = """
            UPDATE blockers
            SET status = 'RESOLVED',
                resolution_summary = ?,
                resolved_at = CURRENT_TIMESTAMP
            WHERE id = ?
        """
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(query, (resolution_summary, blocker_id))
            await db.commit()

        resolved = await self.get_blocker(blocker_id)
        if not resolved:
            raise ResourceNotFoundError("Blocker", blocker_id)
        return resolved

    # --- Audit Trail & Reflections ---

    async def record_audit_event(
        self,
        project_id: str,
        session_id: str | None,
        agent_id: str,
        tool_name: str,
        action: str,
        target_resource: str | None,
        success: bool,
        execution_duration_ms: int,
        error_code: str | None = None,
        payload_summary: dict[str, Any] | None = None,
    ) -> int:
        """Appends an event to the audit trail."""
        query = """
            INSERT INTO audit_log
            (project_id, session_id, agent_id, tool_name, action, target_resource, success, error_code, payload_summary, execution_duration_ms)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """
        summary_json = json.dumps(payload_summary) if payload_summary else None
        async with aiosqlite.connect(self.db_path) as db:
            cursor = await db.execute(
                query,
                (
                    project_id,
                    session_id,
                    agent_id,
                    tool_name,
                    action,
                    target_resource,
                    1 if success else 0,
                    error_code,
                    summary_json,
                    execution_duration_ms,
                ),
            )
            await db.commit()
            return cursor.lastrowid or 0

    async def get_audit_events(
        self,
        project_id: str,
        limit: int = 50,
    ) -> list[dict[str, Any]]:
        """Retrieves recent audit log events."""
        query = "SELECT * FROM audit_log WHERE project_id = ? ORDER BY event_id DESC LIMIT ?"
        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(query, (project_id, limit)) as cursor:
                rows = await cursor.fetchall()
                results = []
                for row in rows:
                    item = dict(row)
                    if item.get("payload_summary"):
                        item["payload_summary"] = json.loads(item["payload_summary"])
                    results.append(item)
                return results

    async def record_reflection(
        self,
        reflection_id: str,
        project_id: str,
        session_id: str,
        agent_id: str,
        completed_tasks_count: int,
        summary: str,
        stale_tasks_identified: list[str] | None = None,
        recommendations: list[str] | None = None,
    ) -> str:
        """Records a workflow reflection."""
        query = """
            INSERT INTO reflections
            (id, project_id, session_id, agent_id, completed_tasks_count, summary, stale_tasks_identified, recommendations)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute(
                query,
                (
                    reflection_id,
                    project_id,
                    session_id,
                    agent_id,
                    completed_tasks_count,
                    summary,
                    json.dumps(stale_tasks_identified or []),
                    json.dumps(recommendations or []),
                ),
            )
            await db.commit()
        return reflection_id

    async def get_last_reflection(self, project_id: str) -> dict[str, Any] | None:
        """Gets the most recent reflection record for the project."""
        query = "SELECT * FROM reflections WHERE project_id = ? ORDER BY created_at DESC LIMIT 1"
        async with aiosqlite.connect(self.db_path) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute(query, (project_id,)) as cursor:
                row = await cursor.fetchone()
                if not row:
                    return None
                item = dict(row)
                item["stale_tasks_identified"] = json.loads(item["stale_tasks_identified"])
                item["recommendations"] = json.loads(item["recommendations"])
                return item
