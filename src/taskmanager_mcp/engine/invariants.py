"""Deterministic workflow invariant validation engine (Rules 1-7)."""

from collections import deque
from typing import Any

from taskmanager_mcp.errors import (
    CompletionEvidenceMissingError,
    CyclicDependencyError,
    DirtySessionExitError,
    ReflectionRequiredError,
    TaskNotActiveError,
    UnresolvedDependencyError,
)


def validate_task_active_for_modification(
    active_task_uuid: str | None,
    target_task_uuid: str,
) -> None:
    """Rule 1: Progress and modifications require the task to be ACTIVE in the current session."""
    if not active_task_uuid or active_task_uuid != target_task_uuid:
        raise TaskNotActiveError(
            task_uuid=target_task_uuid,
            detail=(
                f"Task '{target_task_uuid}' cannot be modified: "
                f"active session task is '{active_task_uuid}'."
            ),
        )


def validate_completion_evidence(
    task_uuid: str,
    summary: str,
    evidence: str,
    confidence: float,
) -> None:
    """Rule 2: Task completion requires minimum summary, concrete evidence, and confidence."""
    if not summary or len(summary.strip()) < 20:
        raise CompletionEvidenceMissingError(
            task_uuid=task_uuid,
            detail=f"Summary must be at least 20 characters (provided: {len(summary.strip()) if summary else 0}).",
        )

    if not evidence or len(evidence.strip()) < 10:
        raise CompletionEvidenceMissingError(
            task_uuid=task_uuid,
            detail=f"Verification evidence must be at least 10 characters (provided: {len(evidence.strip()) if evidence else 0}).",
        )

    if not (0.0 <= confidence <= 1.0):
        raise CompletionEvidenceMissingError(
            task_uuid=task_uuid,
            detail=f"Confidence must be between 0.0 and 1.0 (provided: {confidence}).",
        )


def validate_clean_session_exit(
    session_id: str,
    active_task_uuid: str | None,
    has_pending_tasks: bool,
    next_action: str | None,
) -> None:
    """Rule 3: Session cannot end with dangling active tasks or unrecorded next actions."""
    if active_task_uuid is not None:
        raise DirtySessionExitError(
            session_id=session_id,
            detail=(
                f"Active task '{active_task_uuid}' must be explicitly completed (task_complete) "
                "or paused before closing the session."
            ),
        )

    if has_pending_tasks:
        if not next_action or len(next_action.strip()) == 0:
            raise DirtySessionExitError(
                session_id=session_id,
                detail="Pending tasks remain in project; a concrete 'next_action' string is required.",
            )


def validate_dependency_acyclic(
    target_uuid: str,
    new_dependency_uuid: str,
    existing_graph: dict[str, list[str]],
) -> None:
    """Rule 6: Prevent circular dependencies on DAG using graph reachability search."""
    if target_uuid == new_dependency_uuid:
        raise CyclicDependencyError(
            task_uuid=target_uuid,
            cycle_path=[target_uuid, target_uuid],
            detail="A task cannot depend on itself.",
        )

    # If target_uuid is reachable from new_dependency_uuid, adding target_uuid -> new_dependency_uuid creates a cycle
    queue: deque[list[str]] = deque([[new_dependency_uuid]])
    visited: set[str] = {new_dependency_uuid}

    while queue:
        path = queue.popleft()
        current = path[-1]

        if current == target_uuid:
            full_cycle = [target_uuid] + path
            raise CyclicDependencyError(
                task_uuid=target_uuid,
                cycle_path=full_cycle,
                detail=f"Cycle detected: {' -> '.join(full_cycle)}.",
            )

        neighbors = existing_graph.get(current, [])
        for neighbor in neighbors:
            if neighbor not in visited:
                visited.add(neighbor)
                queue.append(path + [neighbor])


def validate_dependencies_resolved(
    task_uuid: str,
    dependencies: list[str],
    tasks_map: dict[str, dict[str, Any]],
) -> None:
    """Rule 6: All prerequisite tasks must be in COMPLETED status."""
    unresolved: list[str] = []
    for dep_uuid in dependencies:
        dep_task = tasks_map.get(dep_uuid)
        if not dep_task or dep_task.get("status", "").lower() != "completed":
            unresolved.append(dep_uuid)

    if unresolved:
        raise UnresolvedDependencyError(
            task_uuid=task_uuid,
            unresolved_dependencies=unresolved,
        )


def validate_reflection_threshold(
    completed_count_since_reflection: int,
    threshold: int = 5,
) -> None:
    """Rule 7: Enforces mandatory reflection gate once task completion threshold is reached."""
    if completed_count_since_reflection >= threshold:
        raise ReflectionRequiredError(
            completed_count=completed_count_since_reflection,
            threshold=threshold,
        )
