"""Session domain model and zombie recovery algorithms."""

from datetime import UTC, datetime
from enum import StrEnum

from pydantic import BaseModel, Field


class SessionStatus(StrEnum):
    ACTIVE = "ACTIVE"
    SUSPENDED = "SUSPENDED"
    CLOSED = "CLOSED"
    ZOMBIE = "ZOMBIE"


class Session(BaseModel):
    session_id: str = Field(description="UUIDv4 of the session")
    agent_id: str
    project_id: str
    status: SessionStatus = Field(default=SessionStatus.ACTIVE)
    start_time: datetime
    end_time: datetime | None = None
    active_task_uuid: str | None = None
    next_action: str | None = None
    files_modified_count: int = 0
    tests_run_count: int = 0
    commits_count: int = 0
    tasks_completed_count: int = 0
    observations_count: int = 0
    last_heartbeat: datetime
    created_at: datetime


def is_session_zombie(session: Session, threshold_seconds: float = 300.0) -> bool:
    """Returns True if an ACTIVE session's heartbeat has lapsed beyond the stale threshold."""
    if session.status != SessionStatus.ACTIVE:
        return False

    now = datetime.now(UTC)
    heartbeat = session.last_heartbeat
    if heartbeat.tzinfo is None:
        heartbeat = heartbeat.replace(tzinfo=UTC)

    delta = (now - heartbeat).total_seconds()
    return delta > threshold_seconds
