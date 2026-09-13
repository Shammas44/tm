from datetime import UTC, datetime, timedelta

import pytest
from pydantic import ValidationError

from taskmanager_mcp.domain.blocker import Blocker, BlockerSeverity, BlockerStatus
from taskmanager_mcp.domain.observation import Observation
from taskmanager_mcp.domain.session import Session, SessionStatus, is_session_zombie
from taskmanager_mcp.domain.task import Task, TaskPriority, TaskStatus


def test_task_domain_validation():
    task = Task(
        uuid="4bf77239-4a7b-4ee9-b4f0-bc37042a969f",
        title="Implement parser table generator",
        description="LR(1) table generator for grammar",
        status=TaskStatus.PENDING,
        priority=TaskPriority.HIGH,
        component="parser",
        confidence=0.9,
        dependencies=["dep-uuid-1"],
        tags=["compiler", "frontend"],
        created_at=datetime.now(UTC),
    )
    assert task.status == TaskStatus.PENDING
    assert task.priority == TaskPriority.HIGH
    assert task.confidence == 0.9
    assert len(task.dependencies) == 1

    # Test confidence validation: out of bounds
    with pytest.raises(ValidationError):
        Task(
            uuid="123",
            title="Invalid",
            description="desc",
            status=TaskStatus.PENDING,
            priority=TaskPriority.LOW,
            component="parser",
            confidence=1.5,
            created_at=datetime.now(UTC),
        )


def test_session_domain_and_zombie_detection():
    now = datetime.now(UTC)
    sess = Session(
        session_id="sess-100",
        agent_id="agent-01",
        project_id="compiler",
        status=SessionStatus.ACTIVE,
        start_time=now,
        last_heartbeat=now,
        created_at=now,
    )
    # Heartbeat just happened -> not zombie
    assert is_session_zombie(sess, threshold_seconds=300) is False

    # Stale heartbeat -> 10 minutes ago
    sess_stale = Session(
        session_id="sess-101",
        agent_id="agent-01",
        project_id="compiler",
        status=SessionStatus.ACTIVE,
        start_time=now - timedelta(minutes=20),
        last_heartbeat=now - timedelta(minutes=10),
        created_at=now - timedelta(minutes=20),
    )
    assert is_session_zombie(sess_stale, threshold_seconds=300) is True


def test_observation_domain_validation():
    obs = Observation(
        id="obs-100",
        project_id="compiler",
        session_id="sess-100",
        author_agent="agent-01",
        topic="Precedence climbing",
        content="Climbing algorithm avoids shift/reduce conflicts.",
        confidence=0.85,
        component="parser",
        code_references=["src/parser/climb.rs:50"],
        created_at=datetime.now(UTC),
    )
    assert obs.confidence == 0.85
    assert len(obs.code_references) == 1


def test_blocker_domain_validation():
    blocker = Blocker(
        id="block-100",
        project_id="compiler",
        session_id="sess-100",
        task_uuid="task-01",
        reason="Third-party crate compilation failure on macOS",
        severity=BlockerSeverity.HIGH,
        status=BlockerStatus.ACTIVE,
        created_at=datetime.now(UTC),
    )
    assert blocker.severity == BlockerSeverity.HIGH
    assert blocker.status == BlockerStatus.ACTIVE
