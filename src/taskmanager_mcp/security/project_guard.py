"""Project directory sandboxing and path traversal confinement."""

import re
from pathlib import Path

from taskmanager_mcp.errors import ProjectMismatchError

PROJECT_SLUG_REGEX = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_-]*$")


def resolve_project_dir(projects_root: Path, project_slug: str) -> Path:
    """Safely resolves and confines a project directory within the authorized projects root.

    Raises ProjectMismatchError if the slug is invalid or contains path traversal attacks.
    """
    if not project_slug or not PROJECT_SLUG_REGEX.match(project_slug):
        raise ProjectMismatchError(
            requested_project=project_slug,
            token_project="valid-slug",
            detail=(
                f"Invalid project slug '{project_slug}'. Must be alphanumeric with hyphens or underscores."
            ),
        )

    root = projects_root.resolve()
    target_dir = (root / project_slug).resolve()

    # Verify target directory is strictly contained inside projects root
    try:
        target_dir.relative_to(root)
    except ValueError:
        raise ProjectMismatchError(
            requested_project=project_slug,
            token_project=str(root),
            detail=f"Security violation: path traversal detected for project '{project_slug}'.",
        ) from None

    return target_dir
