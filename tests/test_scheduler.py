from taskmanager_mcp.engine.scheduler import select_next_executable_task


def test_select_next_executable_task_basic():
    tasks = [
        {
            "uuid": "task-low",
            "description": "Low priority work",
            "status": "pending",
            "priority": "L",
            "urgency": 2.0,
            "depends": [],
            "component": "docs",
        },
        {
            "uuid": "task-high",
            "description": "Critical security patch",
            "status": "pending",
            "priority": "H",
            "urgency": 12.0,
            "depends": [],
            "component": "auth",
        },
    ]
    active_blocker_task_uuids = set()

    next_task = select_next_executable_task(
        tasks=tasks,
        active_blocker_uuids=active_blocker_task_uuids,
    )
    assert next_task is not None
    assert next_task["uuid"] == "task-high"


def test_select_next_skips_unresolved_dependencies_and_blockers():
    tasks = [
        {
            "uuid": "task-high-blocked",
            "description": "High prio task with unresolved dependency",
            "status": "pending",
            "priority": "H",
            "urgency": 15.0,
            "depends": ["task-dep-1"],
            "component": "core",
        },
        {
            "uuid": "task-high-active-blocker",
            "description": "High prio task blocked by blocker record",
            "status": "pending",
            "priority": "H",
            "urgency": 14.0,
            "depends": [],
            "component": "core",
        },
        {
            "uuid": "task-ready",
            "description": "Medium prio task ready to run",
            "status": "pending",
            "priority": "M",
            "urgency": 8.0,
            "depends": [],
            "component": "core",
        },
    ]
    active_blocker_task_uuids = {"task-high-active-blocker"}

    next_task = select_next_executable_task(
        tasks=tasks,
        active_blocker_uuids=active_blocker_task_uuids,
    )
    assert next_task is not None
    assert next_task["uuid"] == "task-ready"


def test_select_next_with_component_filter():
    tasks = [
        {
            "uuid": "task-lexer",
            "description": "Lexer task",
            "status": "pending",
            "priority": "M",
            "urgency": 8.0,
            "depends": [],
            "component": "lexer",
        },
        {
            "uuid": "task-codegen",
            "description": "Codegen task",
            "status": "pending",
            "priority": "H",
            "urgency": 12.0,
            "depends": [],
            "component": "codegen",
        },
    ]

    next_task = select_next_executable_task(
        tasks=tasks,
        active_blocker_uuids=set(),
        component="lexer",
    )
    assert next_task is not None
    assert next_task["uuid"] == "task-lexer"


def test_select_next_empty():
    assert select_next_executable_task([], set()) is None
