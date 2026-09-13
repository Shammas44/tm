"""Blocker domain model representing explicit dependency and architectural blocks."""

from datetime import datetime
from enum import StrEnum

from pydantic import BaseModel, Field


class BlockerSeverity(StrEnum):
    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"
    CRITICAL = "CRITICAL"


class BlockerStatus(StrEnum):
    ACTIVE = "ACTIVE"
    RESOLVED = "RESOLVED"


class Blocker(BaseModel):
    id: str = Field(description="UUIDv4 identifier of the blocker")
    project_id: str
    session_id: str
    task_uuid: str
    blocking_task_uuid: str | None = None
    reason: str = Field(min_length=5)
    severity: BlockerSeverity = Field(default=BlockerSeverity.HIGH)
    status: BlockerStatus = Field(default=BlockerStatus.ACTIVE)
    resolution_summary: str | None = None
    created_at: datetime
    resolved_at: datetime | None = None
