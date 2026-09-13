from pathlib import Path

import pytest

from taskmanager_mcp.storage.taskwarrior_bridge import TaskwarriorBridge


@pytest.fixture
def bridge(tmp_path: Path):
    project_dir = tmp_path / "test_project"
    return TaskwarriorBridge(project_dir=project_dir)


@pytest.mark.asyncio
async def test_taskwarrior_initialization(bridge: TaskwarriorBridge):
    assert bridge.taskrc_path.exists()
    assert bridge.data_dir.exists()
    assert bridge.lock_path.exists()

    content = bridge.taskrc_path.read_text()
    assert "uda.component.type=string" in content
    assert "uda.confidence.type=numeric" in content
    assert "data.location=" in content


@pytest.mark.asyncio
async def test_create_and_get_task(bridge: TaskwarriorBridge):
    created = await bridge.add_task(
        title="Implement lexer module",
        component="lexer",
        priority="H",
        tags=["compiler", "frontend"],
        estimate_minutes=120,
        confidence=0.95,
        owner="agent-01",
    )
    assert created["uuid"] is not None
    assert created["description"] == "Implement lexer module"
    assert created["component"] == "lexer"
    assert created["priority"] == "H"
    assert "compiler" in created.get("tags", [])

    fetched = await bridge.get_task(created["uuid"])
    assert fetched is not None
    assert fetched["uuid"] == created["uuid"]
    assert fetched["status"] == "pending"


@pytest.mark.asyncio
async def test_modify_and_annotate_task(bridge: TaskwarriorBridge):
    task = await bridge.add_task(
        title="Refactor AST nodes",
        component="ast",
        priority="M",
    )
    uuid = task["uuid"]

    # Annotate
    annotated = await bridge.annotate_task(uuid, "Initial AST profiling completed.")
    assert "annotations" in annotated
    assert len(annotated["annotations"]) >= 1
    assert "Initial AST profiling" in annotated["annotations"][-1]["description"]

    # Modify priority and evidence
    updated = await bridge.modify_task(uuid, priority="L", evidence="pytest test_ast.py passed")
    assert updated["priority"] == "L"
    assert updated["evidence"] == "pytest test_ast.py passed"


@pytest.mark.asyncio
async def test_dependencies(bridge: TaskwarriorBridge):
    parent = await bridge.add_task(title="Code generation", component="codegen")
    child = await bridge.add_task(title="Instruction selection", component="codegen")

    await bridge.add_dependency(parent["uuid"], child["uuid"])

    fetched_parent = await bridge.get_task(parent["uuid"])
    assert child["uuid"] in fetched_parent.get("depends", [])

    await bridge.remove_dependency(parent["uuid"], child["uuid"])
    fetched_parent = await bridge.get_task(parent["uuid"])
    assert child["uuid"] not in fetched_parent.get("depends", [])


@pytest.mark.asyncio
async def test_complete_and_delete_task(bridge: TaskwarriorBridge):
    task = await bridge.add_task(title="Test completion", component="ci")
    uuid = task["uuid"]

    completed = await bridge.complete_task(uuid)
    assert completed["status"] == "completed"

    # Delete task
    task2 = await bridge.add_task(title="Temporary task", component="ci")
    deleted = await bridge.delete_task(task2["uuid"])
    assert deleted is True
