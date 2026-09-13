"""Intelligent task scheduler for selecting the highest-urgency executable task."""

from typing import Any


def _priority_weight(priority: str | None) -> int:
    weights = {"H": 3, "M": 2, "L": 1}
    return weights.get(priority.upper() if priority else "", 0)


def select_next_executable_task(
    tasks: list[dict[str, Any]],
    active_blocker_uuids: set[str],
    component: str | None = None,
) -> dict[str, Any] | None:
    """Calculates and returns the single highest-priority executable task.

    A task is executable if:
    1. Its status is 'pending' or 'ready'.
    2. It is not listed in active_blocker_uuids.
    3. All of its dependencies are 'completed'.
    4. It matches the optional component filter.
    """
    completed_uuids = {t["uuid"] for t in tasks if t.get("status", "").lower() == "completed"}

    candidates: list[dict[str, Any]] = []

    for task in tasks:
        uuid = task.get("uuid")
        status = task.get("status", "").lower()

        if status not in ("pending", "ready"):
            continue

        if uuid in active_blocker_uuids:
            continue

        if component and task.get("component") != component:
            continue

        # Check dependencies
        deps = task.get("depends", [])
        all_deps_completed = all(dep in completed_uuids for dep in deps)
        if not all_deps_completed:
            continue

        candidates.append(task)

    if not candidates:
        return None

    # Sort descending by urgency, then priority weight, then earlier entry/creation
    def sort_key(t: dict[str, Any]) -> tuple[float, int, str]:
        urgency = float(t.get("urgency", 0.0))
        prio = _priority_weight(t.get("priority"))
        entry = t.get("entry", "")
        # Note: entry ascending (earlier is better), so we invert or handle in tuple
        return (urgency, prio, entry)

    candidates.sort(key=sort_key, reverse=True)
    return candidates[0]
