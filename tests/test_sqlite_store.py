from pathlib import Path

import pytest

from taskmanager_mcp.storage.sqlite_store import SQLiteMemoryStore


@pytest.fixture
def store(tmp_path: Path):
    db_path = tmp_path / "test_project" / "memory.db"
    return SQLiteMemoryStore(db_path=db_path)


@pytest.mark.asyncio
async def test_session_lifecycle_in_sqlite(store: SQLiteMemoryStore):
    await store.initialize()

    # Create session
    session = await store.create_session(
        session_id="sess-01",
        agent_id="agent-claude",
        project_id="compiler",
    )
    assert session["session_id"] == "sess-01"
    assert session["status"] == "ACTIVE"

    # Heartbeat
    updated = await store.heartbeat_session("sess-01")
    assert updated is True

    # Update active task
    await store.update_session("sess-01", active_task_uuid="task-uuid-123")
    fetched = await store.get_session("sess-01")
    assert fetched["active_task_uuid"] == "task-uuid-123"

    # Close session
    closed = await store.close_session(
        session_id="sess-01",
        next_action="Run benchmark tests",
        files_modified_count=3,
        tests_run_count=12,
        commits_count=1,
    )
    assert closed["status"] == "CLOSED"
    assert closed["next_action"] == "Run benchmark tests"
    assert closed["files_modified_count"] == 3


@pytest.mark.asyncio
async def test_observations_and_fts5_search(store: SQLiteMemoryStore):
    await store.initialize()
    await store.create_session("sess-02", "agent-02", "compiler")

    obs_id = await store.record_observation(
        observation_id="obs-01",
        project_id="compiler",
        session_id="sess-02",
        author_agent="agent-02",
        topic="Lexer Cache Invalidation",
        content="Incremental parsing cache fails to clear on multiline string tokens.",
        confidence=0.92,
        component="lexer",
        code_references=["src/lexer/cache.rs:45"],
    )
    assert obs_id == "obs-01"

    # Search with FTS5
    results = await store.search_observations(project_id="compiler", query="multiline string")
    assert len(results) == 1
    assert results[0]["id"] == "obs-01"
    assert results[0]["topic"] == "Lexer Cache Invalidation"
    assert "src/lexer/cache.rs:45" in results[0]["code_references"]

    # Search with no match
    empty_results = await store.search_observations(project_id="compiler", query="quantum computer")
    assert len(empty_results) == 0


@pytest.mark.asyncio
async def test_blockers_crud(store: SQLiteMemoryStore):
    await store.initialize()
    await store.create_session("sess-03", "agent-03", "compiler")

    blocker_id = await store.create_blocker(
        blocker_id="block-01",
        project_id="compiler",
        session_id="sess-03",
        task_uuid="task-target-99",
        blocking_task_uuid="task-prereq-01",
        reason="Upstream API type signature changed",
        severity="HIGH",
    )
    assert blocker_id == "block-01"

    active_blockers = await store.list_active_blockers(
        project_id="compiler", task_uuid="task-target-99"
    )
    assert len(active_blockers) == 1
    assert active_blockers[0]["severity"] == "HIGH"

    # Resolve blocker
    resolved = await store.resolve_blocker(
        blocker_id="block-01",
        resolution_summary="Updated API signatures to match upstream v2.",
    )
    assert resolved["status"] == "RESOLVED"
    assert resolved["resolution_summary"] == "Updated API signatures to match upstream v2."


@pytest.mark.asyncio
async def test_audit_log_and_reflections(store: SQLiteMemoryStore):
    await store.initialize()
    await store.create_session("sess-04", "agent-04", "compiler")

    await store.record_audit_event(
        project_id="compiler",
        session_id="sess-04",
        agent_id="agent-04",
        tool_name="task_complete",
        action="COMPLETE",
        target_resource="task-123",
        success=True,
        execution_duration_ms=15,
        payload_summary={"evidence_len": 40},
    )

    audit_records = await store.get_audit_events(project_id="compiler")
    assert len(audit_records) == 1
    assert audit_records[0]["tool_name"] == "task_complete"

    # Record reflection
    ref_id = await store.record_reflection(
        reflection_id="refl-01",
        project_id="compiler",
        session_id="sess-04",
        agent_id="agent-04",
        completed_tasks_count=5,
        summary="Completed frontend lexical analysis milestone.",
        stale_tasks_identified=["task-old-01"],
        recommendations=["Refactor token streaming"],
    )
    assert ref_id == "refl-01"
