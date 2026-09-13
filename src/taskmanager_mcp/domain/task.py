"""Task domain entity and state representations."""

from datetime import datetime
from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field


class TaskStatus(StrEnum):
    PENDING = "pending"
    READY = "ready"
    ACTIVE = "active"
    BLOCKED = "blocked"
    COMPLETED = "completed"
    DELETED = "deleted"


class TaskPriority(StrEnum):
    HIGH = "H"
    MEDIUM = "M"
    LOW = "L"
    NONE = "NONE"


class TaskAnnotation(BaseModel):
    timestamp: datetime
    author: str | None = None
    summary: str
    evidence: str | None = None
    confidence: float = Field(default=1.0, ge=0.0, le=1.0)


class Task(BaseModel):
    uuid: str = Field(description="Immutable UUIDv4")
    integer_id: int | None = Field(default=None, description="Transient Taskwarrior integer ID")
    title: str
    description: str = Field(default="")
    status: TaskStatus = Field(default=TaskStatus.PENDING)
    priority: TaskPriority = Field(default=TaskPriority.MEDIUM)
    dependencies: list[str] = Field(
        default_factory=list, description="UUIDs of tasks this task depends on"
    )
    tags: list[str] = Field(default_factory=list)
    component: str
    confidence: float = Field(default=1.0, ge=0.0, le=1.0)
    estimate_minutes: int | None = Field(default=None, ge=0)
    owner: str | None = None
    annotations: list[TaskAnnotation] = Field(default_factory=list)
    evidence: str | None = None
    created_at: datetime
    updated_at: datetime | None = None

    @classmethod
    def from_taskwarrior_dict(cls, raw: dict[str, Any]) -> "Task":
        """Factory method translating Taskwarrior exported JSON dictionary to domain Task."""
        # Map raw status
        raw_status = raw.get("status", "pending").lower()
        status_map = {
            "pending": TaskStatus.PENDING,
            "completed": TaskStatus.COMPLETED,
            "deleted": TaskStatus.DELETED,
            "active": TaskStatus.ACTIVE,
            "blocked": TaskStatus.BLOCKED,
        }
        status = status_map.get(raw_status, TaskStatus.PENDING)

        # Parse priority
        raw_prio = raw.get("priority", "M")
        try:
            priority = TaskPriority(raw_prio)
        except ValueError:
            priority = TaskPriority.MEDIUM

        # Parse annotations
        raw_annotations = raw.get("annotations", [])
        annotations = []
        for a in raw_annotations:
            # Taskwarrior annotations typically have 'entry' (timestamp) and 'description'
            entry_str = a.get("entry")
            try:
                ts = datetime.fromisoformat(entry_str) if entry_str else datetime.now()
            except ValueError:
                ts = datetime.now()
            annotations.append(
                TaskAnnotation(
                    timestamp=ts,
                    summary=a.get("description", ""),
                    confidence=float(a.get("confidence", 1.0)),
                )
            )

        # Parse created timestamp
        entry_raw = raw.get("entry")
        try:
            created_at = datetime.fromisoformat(entry_raw) if entry_raw else datetime.now()
        except ValueError:
            created_at = datetime.now()

        # Parse modified timestamp
        mod_raw = raw.get("modified")
        try:
            updated_at = datetime.fromisoformat(mod_raw) if mod_raw else None
        except ValueError:
            updated_at = None

        return cls(
            uuid=raw.get("uuid", ""),
            integer_id=raw.get("id"),
            title=raw.get("description", ""),
            description=raw.get("description", ""),
            status=status,
            priority=priority,
            dependencies=raw.get("depends", []),
            tags=raw.get("tags", []),
            component=raw.get("component", "general"),
            confidence=float(raw.get("confidence", 1.0)),
            estimate_minutes=int(raw.get("estimate_minutes"))
            if raw.get("estimate_minutes") is not None
            else None,
            owner=raw.get("owner"),
            annotations=annotations,
            evidence=raw.get("evidence"),
            created_at=created_at,
            updated_at=updated_at,
        )
