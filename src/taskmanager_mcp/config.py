"""Configuration settings for TaskManager MCP Server."""

import os
from pathlib import Path

from pydantic import BaseModel, Field


class ServerConfig(BaseModel):
    """Runtime configuration for TaskManager MCP."""

    projects_root: Path = Field(
        default_factory=lambda: Path(
            os.environ.get("TASKMANAGER_PROJECTS_ROOT", "/var/lib/taskmanager/projects")
        )
    )
    jwt_public_key_path: Path | None = Field(
        default_factory=lambda: (
            Path(os.environ["TASKMANAGER_PUBLIC_KEY_PATH"])
            if "TASKMANAGER_PUBLIC_KEY_PATH" in os.environ
            else None
        )
    )
    jwt_public_key_pem: str | None = Field(default=None)
    jwt_secret: str | None = Field(default_factory=lambda: os.environ.get("TASKMANAGER_JWT_SECRET"))
    expected_aud: str = Field(default="taskmanager-mcp")
    expected_iss: str | None = Field(default=None)
    reflection_threshold: int = Field(default=5)
    stale_session_threshold_seconds: float = Field(default=300.0)
    env_auth_token: str | None = Field(
        default_factory=lambda: (
            os.environ.get("TASKMANAGER_AUTH_TOKEN") or os.environ.get("MCP_AUTH_TOKEN")
        )
    )
