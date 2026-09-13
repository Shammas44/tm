"""Observation domain model representing durable engineering knowledge."""

from datetime import datetime

from pydantic import BaseModel, Field


class Observation(BaseModel):
    id: str = Field(description="UUIDv4 identifier of the observation")
    project_id: str
    session_id: str
    author_agent: str
    topic: str = Field(min_length=3)
    content: str = Field(min_length=15)
    confidence: float = Field(default=1.0, ge=0.0, le=1.0)
    component: str
    code_references: list[str] = Field(default_factory=list)
    created_at: datetime
