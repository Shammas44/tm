import pytest

from taskmanager_mcp.engine.invariants import (
    validate_clean_session_exit,
    validate_completion_evidence,
    validate_dependencies_resolved,
    validate_dependency_acyclic,
    validate_reflection_threshold,
    validate_task_active_for_modification,
)
from taskmanager_mcp.errors import (
    CompletionEvidenceMissingError,
    CyclicDependencyError,
    DirtySessionExitError,
    ReflectionRequiredError,
    TaskNotActiveError,
    UnresolvedDependencyError,
)


def test_validate_task_active_for_modification():
    # Pass: active task matches
    validate_task_active_for_modification(
        active_task_uuid="task-01",
        target_task_uuid="task-01",
    )

    # Fail: different active task
    with pytest.raises(TaskNotActiveError):
        validate_task_active_for_modification(
            active_task_uuid="task-02",
            target_task_uuid="task-01",
        )

    # Fail: no active task
    with pytest.raises(TaskNotActiveError):
        validate_task_active_for_modification(
            active_task_uuid=None,
            target_task_uuid="task-01",
        )


def test_validate_completion_evidence():
    # Valid
    validate_completion_evidence(
        task_uuid="task-01",
        summary="Implemented LR(1) lookahead parser generator with unit test coverage.",
        evidence="pytest tests/test_parser.py passed with 14 tests.",
        confidence=0.95,
    )

    # Short summary (< 20 chars)
    with pytest.raises(CompletionEvidenceMissingError) as exc_info:
        validate_completion_evidence(
            task_uuid="task-01",
            summary="Done.",
            evidence="pytest passed successfully.",
            confidence=0.9,
        )
    assert "summary" in str(exc_info.value).lower()

    # Short evidence (< 10 chars)
    with pytest.raises(CompletionEvidenceMissingError) as exc_info:
        validate_completion_evidence(
            task_uuid="task-01",
            summary="Implemented full grammar table serialisation routines.",
            evidence="tested ok",
            confidence=0.9,
        )
    assert "evidence" in str(exc_info.value).lower()


def test_validate_clean_session_exit():
    # Valid exit with no active task and next action provided
    validate_clean_session_exit(
        session_id="sess-01",
        active_task_uuid=None,
        has_pending_tasks=True,
        next_action="Run test suite and benchmark on morning build.",
    )

    # Valid exit with no pending tasks
    validate_clean_session_exit(
        session_id="sess-01",
        active_task_uuid=None,
        has_pending_tasks=False,
        next_action=None,
    )

    # Fail: active task left dangling
    with pytest.raises(DirtySessionExitError):
        validate_clean_session_exit(
            session_id="sess-01",
            active_task_uuid="task-dangling-01",
            has_pending_tasks=True,
            next_action="Some action",
        )

    # Fail: pending work exists but next_action is missing
    with pytest.raises(DirtySessionExitError):
        validate_clean_session_exit(
            session_id="sess-01",
            active_task_uuid=None,
            has_pending_tasks=True,
            next_action="",
        )


def test_validate_dependency_acyclic():
    # Graph: A depends on B (B -> A), B depends on C (C -> B)
    # Adding C depends on A would create cycle A -> B -> C -> A
    dependency_graph = {
        "A": ["B"],
        "B": ["C"],
        "C": [],
    }

    # Valid dependency: C depends on D
    validate_dependency_acyclic(
        target_uuid="C",
        new_dependency_uuid="D",
        existing_graph=dependency_graph,
    )

    # Invalid cyclic dependency: C depends on A
    with pytest.raises(CyclicDependencyError):
        validate_dependency_acyclic(
            target_uuid="C",
            new_dependency_uuid="A",
            existing_graph=dependency_graph,
        )


def test_validate_dependencies_resolved():
    # Pass: all dependencies completed
    tasks_by_uuid = {
        "dep-1": {"status": "completed"},
        "dep-2": {"status": "completed"},
    }
    validate_dependencies_resolved(
        task_uuid="task-target",
        dependencies=["dep-1", "dep-2"],
        tasks_map=tasks_by_uuid,
    )

    # Fail: dep-2 is still pending
    tasks_by_uuid["dep-2"]["status"] = "pending"
    with pytest.raises(UnresolvedDependencyError):
        validate_dependencies_resolved(
            task_uuid="task-target",
            dependencies=["dep-1", "dep-2"],
            tasks_map=tasks_by_uuid,
        )


def test_validate_reflection_threshold():
    # Pass: 3 completed tasks with threshold 5
    validate_reflection_threshold(completed_count_since_reflection=3, threshold=5)

    # Fail: 5 completed tasks with threshold 5
    with pytest.raises(ReflectionRequiredError):
        validate_reflection_threshold(completed_count_since_reflection=5, threshold=5)
