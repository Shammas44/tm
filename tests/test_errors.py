from taskmanager_mcp.errors import (
    AuthenticationError,
    CompletionEvidenceMissingError,
    InvariantViolationError,
    StorageLockTimeoutError,
    TaskManagerError,
    TaskNotActiveError,
    to_mcp_error_data,
)


def test_exception_hierarchy():
    err = TaskNotActiveError(task_uuid="1234", detail="Task is not active.")
    assert isinstance(err, InvariantViolationError)
    assert isinstance(err, TaskManagerError)
    assert err.error_code == "TASK_NOT_ACTIVE"
    assert err.task_uuid == "1234"
    assert "Task is not active." in str(err)


def test_to_mcp_error_data():
    err = CompletionEvidenceMissingError(
        task_uuid="test-uuid",
        detail="Missing evidence for completion.",
        remedy="Run pytest and capture terminal output.",
    )
    data = to_mcp_error_data(err)
    assert data["error_code"] == "COMPLETION_EVIDENCE_MISSING"
    assert data["retryable"] is False
    assert data["task_uuid"] == "test-uuid"
    assert data["detail"] == "Missing evidence for completion."
    assert data["remedy"] == "Run pytest and capture terminal output."


def test_custom_remedies_and_codes():
    auth_err = AuthenticationError("Token expired")
    assert auth_err.error_code == "UNAUTHENTICATED"
    assert auth_err.retryable is False

    lock_err = StorageLockTimeoutError("Lock busy")
    assert lock_err.error_code == "STORAGE_LOCK_TIMEOUT"
    assert lock_err.retryable is True
