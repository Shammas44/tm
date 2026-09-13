"""Standardized domain exception hierarchy and MCP error serialization for TaskManager MCP."""

from typing import Any


class TaskManagerError(Exception):
    """Base exception for all TaskManager domain and protocol errors."""

    def __init__(
        self,
        message: str,
        error_code: str = "INTERNAL_ERROR",
        retryable: bool = False,
        detail: str | None = None,
        remedy: str | None = None,
        **extra_metadata: Any,
    ):
        super().__init__(message)
        self.message = message
        self.error_code = error_code
        self.retryable = retryable
        self.detail = detail or message
        self.remedy = remedy
        self.extra_metadata = extra_metadata

    def to_dict(self) -> dict[str, Any]:
        """Convert the exception to a structured MCP JSON-RPC data payload."""
        payload: dict[str, Any] = {
            "error_code": self.error_code,
            "retryable": self.retryable,
            "detail": self.detail,
        }
        if self.remedy:
            payload["remedy"] = self.remedy
        for key, value in self.extra_metadata.items():
            if value is not None:
                payload[key] = value
        return payload


class AuthenticationError(TaskManagerError):
    """Raised when authentication fails (missing, invalid, or expired JWT)."""

    def __init__(
        self,
        message: str = "Authentication required or token invalid.",
        detail: str | None = None,
        remedy: str
        | None = "Provide a valid, unexpired JWT with the required audience and claims.",
        **extra: Any,
    ):
        super().__init__(
            message=message,
            error_code="UNAUTHENTICATED",
            retryable=False,
            detail=detail,
            remedy=remedy,
            **extra,
        )


class ForbiddenPermissionError(TaskManagerError):
    """Raised when an authenticated agent lacks the required capability."""

    def __init__(
        self,
        permission: str,
        message: str | None = None,
        detail: str | None = None,
        **extra: Any,
    ):
        msg = message or f"Permission denied. Required capability: '{permission}'."
        remedy = f"Request a token with the '{permission}' permission or an elevated role."
        super().__init__(
            message=msg,
            error_code="FORBIDDEN_PERMISSION",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            required_permission=permission,
            **extra,
        )


class ProjectMismatchError(TaskManagerError):
    """Raised when an agent attempts to access a project outside its token claims."""

    def __init__(
        self,
        requested_project: str,
        token_project: str,
        **extra: Any,
    ):
        msg = (
            f"Project access violation: Token is confined to project '{token_project}', "
            f"cannot access '{requested_project}'."
        )
        remedy = f"Switch to a token issued for project '{requested_project}'."
        super().__init__(
            message=msg,
            error_code="PROJECT_MISMATCH",
            retryable=False,
            detail=msg,
            remedy=remedy,
            requested_project=requested_project,
            token_project=token_project,
            **extra,
        )


class InvariantViolationError(TaskManagerError):
    """Base exception for workflow invariant violations (Rules 1-7)."""

    pass


class TaskNotActiveError(InvariantViolationError):
    """Rule 1: No modifications or progress on tasks that are not ACTIVE."""

    def __init__(
        self,
        task_uuid: str,
        message: str | None = None,
        detail: str | None = None,
        remedy: str
        | None = "Start the task using task_start before recording work or completing it.",
        **extra: Any,
    ):
        msg = (
            message
            or detail
            or f"Task '{task_uuid}' is not currently marked ACTIVE in this session."
        )
        super().__init__(
            message=msg,
            error_code="TASK_NOT_ACTIVE",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            task_uuid=task_uuid,
            **extra,
        )
        self.task_uuid = task_uuid


class CompletionEvidenceMissingError(InvariantViolationError):
    """Rule 2: Task completion requires summary and concrete verification evidence."""

    def __init__(
        self,
        task_uuid: str,
        message: str | None = None,
        detail: str | None = None,
        remedy: str | None = (
            "Provide a detailed summary (>=20 chars) and concrete verification evidence (>=10 chars) "
            "such as test output, command transcripts, or diff hashes."
        ),
        **extra: Any,
    ):
        msg = (
            message
            or detail
            or f"Completion rejected for task '{task_uuid}': mandatory summary or verification evidence missing."
        )
        super().__init__(
            message=msg,
            error_code="COMPLETION_EVIDENCE_MISSING",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            task_uuid=task_uuid,
            **extra,
        )
        self.task_uuid = task_uuid


class DirtySessionExitError(InvariantViolationError):
    """Rule 3: Session cannot end with dangling active tasks or undocumented pending work."""

    def __init__(
        self,
        session_id: str,
        detail: str | None = None,
        remedy: str | None = (
            "Complete or pause the active task, and provide a next_action description for pending work."
        ),
        **extra: Any,
    ):
        msg = f"Session '{session_id}' cannot end: active task remains uncompleted or next_action is missing."
        super().__init__(
            message=msg,
            error_code="DIRTY_SESSION_EXIT_REJECTED",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            session_id=session_id,
            **extra,
        )
        self.session_id = session_id


class ReflectionRequiredError(InvariantViolationError):
    """Rule 7: Mandatory reflection required after reaching task threshold."""

    def __init__(
        self,
        completed_count: int,
        threshold: int,
        detail: str | None = None,
        remedy: str
        | None = "Invoke workflow_reflect to analyze completed work and identify technical debt.",
        **extra: Any,
    ):
        msg = f"Reflection threshold exceeded: {completed_count} tasks completed (threshold: {threshold})."
        super().__init__(
            message=msg,
            error_code="REFLECTION_REQUIRED",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            completed_count=completed_count,
            threshold=threshold,
            **extra,
        )


class CyclicDependencyError(InvariantViolationError):
    """Rule 6: Dependency DAG cannot contain cycles."""

    def __init__(
        self,
        task_uuid: str,
        cycle_path: list[str],
        detail: str | None = None,
        remedy: str | None = "Remove the cyclic dependency link to preserve an acyclic task graph.",
        **extra: Any,
    ):
        msg = f"Cyclic dependency detected involving task '{task_uuid}': {' -> '.join(cycle_path)}."
        super().__init__(
            message=msg,
            error_code="CYCLIC_DEPENDENCY",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            task_uuid=task_uuid,
            cycle_path=cycle_path,
            **extra,
        )


class UnresolvedDependencyError(InvariantViolationError):
    """Rule 6: Task cannot start while dependencies remain incomplete."""

    def __init__(
        self,
        task_uuid: str,
        unresolved_dependencies: list[str],
        detail: str | None = None,
        remedy: str | None = "Complete all prerequisite tasks before starting this task.",
        **extra: Any,
    ):
        msg = (
            f"Task '{task_uuid}' has {len(unresolved_dependencies)} unresolved dependencies: "
            f"{', '.join(unresolved_dependencies)}."
        )
        super().__init__(
            message=msg,
            error_code="UNRESOLVED_DEPENDENCY_BLOCK",
            retryable=False,
            detail=detail or msg,
            remedy=remedy,
            task_uuid=task_uuid,
            unresolved_dependencies=unresolved_dependencies,
            **extra,
        )


class ActiveSessionConflictError(TaskManagerError):
    """Raised when an active session is already leased to an agent for the project."""

    def __init__(
        self,
        project: str,
        active_session_id: str,
        last_heartbeat: str,
        **extra: Any,
    ):
        msg = f"Active session '{active_session_id}' is currently leased for project '{project}'."
        remedy = "Wait for active session to end, send heartbeats, or allow stale session timeout to expire."
        super().__init__(
            message=msg,
            error_code="ACTIVE_SESSION_CONFLICT",
            retryable=True,
            detail=msg,
            remedy=remedy,
            active_session_id=active_session_id,
            last_heartbeat=last_heartbeat,
            **extra,
        )


class StorageLockTimeoutError(TaskManagerError):
    """Raised when acquiring file lock on Taskwarrior or SQLite storage times out."""

    def __init__(
        self,
        resource: str,
        timeout_seconds: float = 5.0,
        **extra: Any,
    ):
        msg = f"Storage lock acquisition timeout ({timeout_seconds}s) on resource '{resource}'."
        remedy = "Retry the operation after other concurrent operations complete."
        super().__init__(
            message=msg,
            error_code="STORAGE_LOCK_TIMEOUT",
            retryable=True,
            detail=msg,
            remedy=remedy,
            resource=resource,
            **extra,
        )


class ResourceNotFoundError(TaskManagerError):
    """Raised when a requested task, blocker, observation, or session does not exist."""

    def __init__(
        self,
        resource_type: str,
        resource_id: str,
        **extra: Any,
    ):
        msg = f"{resource_type} with ID '{resource_id}' was not found."
        remedy = f"Verify that the {resource_type.lower()} ID is correct and exists in the current project."
        super().__init__(
            message=msg,
            error_code=f"{resource_type.upper()}_NOT_FOUND",
            retryable=False,
            detail=msg,
            remedy=remedy,
            resource_type=resource_type,
            resource_id=resource_id,
            **extra,
        )


def to_mcp_error_data(err: TaskManagerError) -> dict[str, Any]:
    """Helper to convert any TaskManagerError into structured MCP error data payload."""
    return err.to_dict()
