from pathlib import Path

import pytest

from taskmanager_mcp.config import ServerConfig
from taskmanager_mcp.security.jwt_validator import create_test_keypair, create_test_token
from taskmanager_mcp.server import TaskManagerServer


@pytest.fixture
def test_setup(tmp_path: Path):
    projects_root = tmp_path / "projects"
    projects_root.mkdir()
    private_pem, public_pem = create_test_keypair()

    config = ServerConfig(
        projects_root=projects_root,
        jwt_public_key_pem=public_pem,
        expected_aud="taskmanager-mcp",
        reflection_threshold=2,  # Low threshold for testing reflection gate
        stale_session_threshold_seconds=1.0,  # Fast zombie expiration for test
    )
    server = TaskManagerServer(config=config)
    return server, private_pem, projects_root


@pytest.mark.asyncio
async def test_server_session_and_task_lifecycle(test_setup):
    server, private_pem, _ = test_setup
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-claude",
        project="compiler",
        role="worker",
    )

    # 1. session_begin
    sess_res = await server.session_begin(
        project="compiler", agent_id="agent-claude", auth_token=token
    )
    assert sess_res["success"] is True
    session_id = sess_res["data"]["session_id"]
    assert session_id is not None

    # Heartbeat
    hb_res = await server.session_heartbeat(session_id=session_id, auth_token=token)
    assert hb_res["success"] is True

    # 2. task_create
    task_res = await server.task_create(
        title="Implement type checker for expressions",
        component="typecheck",
        priority="H",
        auth_token=token,
    )
    assert task_res["success"] is True
    task_uuid = task_res["data"]["uuid"]

    # Invariant Rule 1 check: Complete before start must fail with TASK_NOT_ACTIVE
    inv_res = await server.task_complete(
        task_uuid=task_uuid,
        summary="Completed without starting task first.",
        evidence="cargo test passed.",
        auth_token=token,
    )
    assert inv_res["success"] is False
    assert inv_res["error"]["error_code"] == "TASK_NOT_ACTIVE"

    # 3. task_start
    start_res = await server.task_start(task_uuid=task_uuid, auth_token=token)
    assert start_res["success"] is True

    # 4. observation_record
    obs_res = await server.observation_record(
        topic="Type Inference Rules",
        content="Bidirectional typing eliminates need for explicit annotations on local bindings.",
        component="typecheck",
        confidence=0.95,
        code_references=["src/types/infer.rs:100"],
        auth_token=token,
    )
    assert obs_res["success"] is True

    # 5. observation_search
    search_res = await server.observation_search(query="Bidirectional typing", auth_token=token)
    assert search_res["success"] is True
    assert len(search_res["data"]) >= 1

    # Invariant Rule 2 check: Complete with too-short evidence must fail
    short_ev_res = await server.task_complete(
        task_uuid=task_uuid,
        summary="A sufficiently long summary describing the work performed on expressions.",
        evidence="too short",
        auth_token=token,
    )
    assert short_ev_res["success"] is False
    assert short_ev_res["error"]["error_code"] == "COMPLETION_EVIDENCE_MISSING"

    # 6. task_complete (with valid evidence)
    comp_res = await server.task_complete(
        task_uuid=task_uuid,
        summary="Successfully implemented bidirectional type inference algorithm.",
        evidence="cargo test test_infer passed with 28 passing unit tests.",
        confidence=0.98,
        auth_token=token,
    )
    assert comp_res["success"] is True

    # 7. session_end
    end_res = await server.session_end(
        session_id=session_id,
        next_action="Implement pattern matching exhaustiveness checks.",
        auth_token=token,
    )
    assert end_res["success"] is True
    assert end_res["data"]["status"] == "CLOSED"


@pytest.mark.asyncio
async def test_subtask_and_scheduler_and_reflection(test_setup):
    server, private_pem, _ = test_setup
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-claude",
        project="compiler",
        role="worker",
    )

    await server.session_begin(project="compiler", agent_id="agent-claude", auth_token=token)

    # Create parent task
    parent_res = await server.task_create(
        title="Frontend parser", component="parser", priority="H", auth_token=token
    )
    parent_uuid = parent_res["data"]["uuid"]

    # Create subtask (parent depends on child)
    child_res = await server.task_create_subtask(
        parent_task_uuid=parent_uuid, title="Lexical scanner", auth_token=token
    )
    assert child_res["success"] is True
    child_uuid = child_res["data"]["uuid"]

    # Scheduler: parent cannot run because child is pending! Child must be selected!
    next_res = await server.task_get_next(auth_token=token)
    assert next_res["success"] is True
    assert next_res["data"]["uuid"] == child_uuid

    # Start and complete child
    await server.task_start(task_uuid=child_uuid, auth_token=token)
    await server.task_complete(
        task_uuid=child_uuid,
        summary="Lexical scanner is complete with full token stream tests.",
        evidence="cargo test test_scanner passed 100%.",
        auth_token=token,
    )

    # Now parent is executable!
    next_res2 = await server.task_get_next(auth_token=token)
    assert next_res2["success"] is True
    assert next_res2["data"]["uuid"] == parent_uuid

    # Start and complete parent (this is the 2nd completed task -> hits reflection threshold=2)
    await server.task_start(task_uuid=parent_uuid, auth_token=token)
    await server.task_complete(
        task_uuid=parent_uuid,
        summary="Frontend parser implementation finished and integrated.",
        evidence="cargo test test_parser passed 100%.",
        auth_token=token,
    )

    # Now task_get_next should fail with REFLECTION_REQUIRED!
    refl_gate = await server.task_get_next(auth_token=token)
    assert refl_gate["success"] is False
    assert refl_gate["error"]["error_code"] == "REFLECTION_REQUIRED"

    # Invoke workflow_reflect
    ref_res = await server.workflow_reflect(
        summary="Completed frontend milestone: scanner and parser are now operational.",
        recommendations=["Investigate AST visitor pattern performance."],
        auth_token=token,
    )
    assert ref_res["success"] is True
    assert ref_res["data"]["reflection_gate_cleared"] is True


@pytest.mark.asyncio
async def test_server_blocker_lifecycle(test_setup):
    server, private_pem, _ = test_setup
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-claude",
        project="compiler",
        role="worker",
    )

    await server.session_begin(project="compiler", agent_id="agent-claude", auth_token=token)

    task_res = await server.task_create(
        title="LLVM IR generation", component="codegen", auth_token=token
    )
    task_uuid = task_res["data"]["uuid"]

    # Create blocker
    blocker_res = await server.blocker_create(
        task_uuid=task_uuid,
        reason="LLVM 18 bindings compatibility broken on arm64",
        severity="CRITICAL",
        auth_token=token,
    )
    assert blocker_res["success"] is True
    blocker_id = blocker_res["data"]["blocker_id"]

    # Resolve blocker
    resolve_res = await server.blocker_resolve(
        blocker_id=blocker_id,
        resolution_summary="Patched LLVM 18 wrapper crate for arm64 target.",
        auth_token=token,
    )
    assert resolve_res["success"] is True
    assert resolve_res["data"]["status"] == "RESOLVED"
