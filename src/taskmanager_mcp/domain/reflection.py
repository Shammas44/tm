"""Reflection domain model representing periodic engineering retrospectives."""

from datetime import datetime

from pydantic import BaseModel, Field


class Reflection(BaseModel):
    id: str = Field(description="UUIDv4 identifier of the reflection")
    project_id: str
    session_id: str
    agent_id: str
    completed_tasks_count: int
    summary: str = Field(min_length=10)
    stale_tasks_identified: list[str] = Field(default_factory=list)
    recommendations: list[str] = Field(default_factory=list)
    created_at: datetime
