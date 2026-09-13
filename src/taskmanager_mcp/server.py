"""TaskManager MCP Server implementation over stdio transport."""

import time
import uuid
from typing import Any

from mcp.server.mcpserver import MCPServer

from taskmanager_mcp.config import ServerConfig
from taskmanager_mcp.domain.session import Session, SessionStatus, is_session_zombie
from taskmanager_mcp.engine.invariants import (
    validate_clean_session_exit,
    validate_completion_evidence,
    validate_dependencies_resolved,
    validate_dependency_acyclic,
    validate_reflection_threshold,
    validate_task_active_for_modification,
)
from taskmanager_mcp.engine.scheduler import select_next_executable_task
from taskmanager_mcp.errors import (
    ActiveSessionConflictError,
    ProjectMismatchError,
    ResourceNotFoundError,
    TaskManagerError,
    to_mcp_error_data,
)
from taskmanager_mcp.security.jwt_validator import JWTValidator, SecurityContext
from taskmanager_mcp.security.project_guard import resolve_project_dir
from taskmanager_mcp.storage.sqlite_store import SQLiteMemoryStore
from taskmanager_mcp.storage.taskwarrior_bridge import TaskwarriorBridge
from taskmanager_mcp.utils.logger import get_logger

logger = get_logger("taskmanager_server")


class ProjectContext:
    """Manages storage and state bindings for an isolated project directory."""

    def __init__(self, project_dir: Any):
        self.project_dir = project_dir
        self.bridge = TaskwarriorBridge(project_dir)
        self.store = SQLiteMemoryStore(project_dir / "memory.db")
        self._initialized = False

    async def ensure_initialized(self) -> None:
        if not self._initialized:
            await self.store.initialize()
            self._initialized = True


class TaskManagerServer:
    """Production MCP Server for agent task memory and workflow governance."""

    def __init__(self, config: ServerConfig | None = None):
        self.config = config or ServerConfig()
        self.validator = JWTValidator(
            public_key_path=self.config.jwt_public_key_path,
            public_key_pem=self.config.jwt_public_key_pem,
            secret=self.config.jwt_secret,
            expected_aud=self.config.expected_aud,
            expected_iss=self.config.expected_iss,
        )
        self._projects: dict[str, ProjectContext] = {}
        self.mcp_server = MCPServer("taskmanager-mcp")
        self._register_mcp_tools()

    async def _get_project_context(self, project: str) -> ProjectContext:
        """Retrieves or creates a cached ProjectContext for an authorized project."""
        if project not in self._projects:
            project_dir = resolve_project_dir(self.config.projects_root, project)
            ctx = ProjectContext(project_dir)
            await ctx.ensure_initialized()
            self._projects[project] = ctx
        else:
            await self._projects[project].ensure_initialized()
        return self._projects[project]

    async def _authenticate(
        self,
        auth_token: str | None,
        required_permission: str,
        project: str | None = None,
    ) -> tuple[SecurityContext, ProjectContext]:
        """Authenticates token, validates RBAC and project confinement."""
        token = auth_token or self.config.env_auth_token
        ctx = self.validator.verify_token(token or "")
        ctx.enforce_permission(required_permission)

        if project and project != ctx.project:
            raise ProjectMismatchError(requested_project=project, token_project=ctx.project)

        proj_ctx = await self._get_project_context(ctx.project)
        return ctx, proj_ctx

    def _register_mcp_tools(self) -> None:
        """Registers all tools with the MCPServer instance."""
        s = self.mcp_server

        @s.tool()
        async def session_begin(
            project: str, agent_id: str, auth_token: str | None = None
        ) -> dict[str, Any]:
            """Initiates an authenticated agent session and restores context."""
            return await self.session_begin(
                project=project, agent_id=agent_id, auth_token=auth_token
            )

        @s.tool()
        async def session_heartbeat(
            session_id: str, auth_token: str | None = None
        ) -> dict[str, Any]:
            """Refreshes the session heartbeat timestamp."""
            return await self.session_heartbeat(session_id=session_id, auth_token=auth_token)

        @s.tool()
        async def session_end(
            session_id: str,
            next_action: str | None = None,
            files_modified_count: int = 0,
            tests_run_count: int = 0,
            commits_count: int = 0,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Safely validates invariants and closes an active session."""
            return await self.session_end(
                session_id=session_id,
                next_action=next_action,
                files_modified_count=files_modified_count,
                tests_run_count=tests_run_count,
                commits_count=commits_count,
                auth_token=auth_token,
            )

        @s.tool()
        async def task_get_next(
            component: str | None = None, auth_token: str | None = None
        ) -> dict[str, Any]:
            """Returns the highest-urgency executable task."""
            return await self.task_get_next(component=component, auth_token=auth_token)

        @s.tool()
        async def task_start(task_uuid: str, auth_token: str | None = None) -> dict[str, Any]:
            """Transitions a task to ACTIVE for the calling session."""
            return await self.task_start(task_uuid=task_uuid, auth_token=auth_token)

        @s.tool()
        async def task_create(
            title: str,
            component: str,
            description: str = "",
            priority: str = "M",
            dependencies: list[str] | None = None,
            estimate_minutes: int | None = None,
            confidence: float = 1.0,
            tags: list[str] | None = None,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Creates a new task with DAG dependency verification."""
            return await self.task_create(
                title=title,
                component=component,
                description=description,
                priority=priority,
                dependencies=dependencies,
                estimate_minutes=estimate_minutes,
                confidence=confidence,
                tags=tags,
                auth_token=auth_token,
            )

        @s.tool()
        async def task_create_subtask(
            parent_task_uuid: str,
            title: str,
            description: str = "",
            estimate_minutes: int | None = None,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Creates a child task and links parent dependency."""
            return await self.task_create_subtask(
                parent_task_uuid=parent_task_uuid,
                title=title,
                description=description,
                estimate_minutes=estimate_minutes,
                auth_token=auth_token,
            )

        @s.tool()
        async def task_complete(
            task_uuid: str,
            summary: str,
            evidence: str,
            confidence: float = 1.0,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Completes an active task with mandatory summary and verification evidence."""
            return await self.task_complete(
                task_uuid=task_uuid,
                summary=summary,
                evidence=evidence,
                confidence=confidence,
                auth_token=auth_token,
            )

        @s.tool()
        async def task_annotate(
            task_uuid: str, summary: str, auth_token: str | None = None
        ) -> dict[str, Any]:
            """Attaches a progress note to an active task."""
            return await self.task_annotate(
                task_uuid=task_uuid, summary=summary, auth_token=auth_token
            )

        @s.tool()
        async def task_reprioritize(
            task_uuid: str, priority: str, auth_token: str | None = None
        ) -> dict[str, Any]:
            """Changes the priority of a task."""
            return await self.task_reprioritize(
                task_uuid=task_uuid, priority=priority, auth_token=auth_token
            )

        @s.tool()
        async def task_get(task_uuid: str, auth_token: str | None = None) -> dict[str, Any]:
            """Retrieves full details for a task."""
            return await self.task_get(task_uuid=task_uuid, auth_token=auth_token)

        @s.tool()
        async def task_list(
            status: str | None = None,
            component: str | None = None,
            tags: list[str] | None = None,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Lists tasks matching filters."""
            return await self.task_list(
                status=status, component=component, tags=tags, auth_token=auth_token
            )

        @s.tool()
        async def observation_record(
            topic: str,
            content: str,
            component: str,
            confidence: float = 1.0,
            code_references: list[str] | None = None,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Persists durable knowledge into the project memory and FTS index."""
            return await self.observation_record(
                topic=topic,
                content=content,
                component=component,
                confidence=confidence,
                code_references=code_references,
                auth_token=auth_token,
            )

        @s.tool()
        async def observation_search(
            query: str,
            component: str | None = None,
            min_confidence: float = 0.0,
            limit: int = 10,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Searches durable observations via full-text search with BM25 ranking."""
            return await self.observation_search(
                query=query,
                component=component,
                min_confidence=min_confidence,
                limit=limit,
                auth_token=auth_token,
            )

        @s.tool()
        async def blocker_create(
            task_uuid: str,
            reason: str,
            severity: str = "HIGH",
            blocking_task_uuid: str | None = None,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Registers an architectural blocker and marks the task as BLOCKED."""
            return await self.blocker_create(
                task_uuid=task_uuid,
                reason=reason,
                severity=severity,
                blocking_task_uuid=blocking_task_uuid,
                auth_token=auth_token,
            )

        @s.tool()
        async def blocker_resolve(
            blocker_id: str,
            resolution_summary: str,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Resolves a blocker and unblocks the affected task."""
            return await self.blocker_resolve(
                blocker_id=blocker_id,
                resolution_summary=resolution_summary,
                auth_token=auth_token,
            )

        @s.tool()
        async def workflow_reflect(
            summary: str,
            stale_tasks_identified: list[str] | None = None,
            recommendations: list[str] | None = None,
            auth_token: str | None = None,
        ) -> dict[str, Any]:
            """Performs mandatory engineering reflection, clearing the reflection gate."""
            return await self.workflow_reflect(
                summary=summary,
                stale_tasks_identified=stale_tasks_identified,
                recommendations=recommendations,
                auth_token=auth_token,
            )

    # --- Tool Implementations ---

    async def session_begin(
        self, project: str, agent_id: str, auth_token: str | None = None
    ) -> dict[str, Any]:
        start_time = time.monotonic()
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "session:start", project)

            # Check existing active session
            active_session = await proj_ctx.store.get_active_session(sec_ctx.project)
            if active_session:
                sess_obj = Session(**active_session)
                if is_session_zombie(sess_obj, self.config.stale_session_threshold_seconds):
                    logger.warning("reaping_zombie_session", session_id=sess_obj.session_id)
                    await proj_ctx.store.update_session(
                        sess_obj.session_id, status=SessionStatus.ZOMBIE
                    )
                else:
                    raise ActiveSessionConflictError(
                        project=sec_ctx.project,
                        active_session_id=sess_obj.session_id,
                        last_heartbeat=str(sess_obj.last_heartbeat),
                    )

            session_id = str(uuid.uuid4())
            new_session = await proj_ctx.store.create_session(
                session_id, sec_ctx.agent_id, sec_ctx.project
            )

            # Fetch context
            active_blockers = await proj_ctx.store.list_active_blockers(sec_ctx.project)
            recent_obs = await proj_ctx.store.search_observations(
                sec_ctx.project, query="*", limit=5
            )

            data = {
                "session_id": session_id,
                "project": sec_ctx.project,
                "agent_id": sec_ctx.agent_id,
                "active_task": None,
                "active_blockers": active_blockers,
                "recent_observations": recent_obs,
                "previous_session_next_action": new_session.get("next_action"),
            }

            duration_ms = int((time.monotonic() - start_time) * 1000)
            await proj_ctx.store.record_audit_event(
                project_id=sec_ctx.project,
                session_id=session_id,
                agent_id=sec_ctx.agent_id,
                tool_name="session_begin",
                action="BEGIN",
                target_resource=session_id,
                success=True,
                execution_duration_ms=duration_ms,
            )
            return {"success": True, "data": data}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def session_heartbeat(
        self, session_id: str, auth_token: str | None = None
    ) -> dict[str, Any]:
        try:
            _, proj_ctx = await self._authenticate(auth_token, "session:start")
            updated = await proj_ctx.store.heartbeat_session(session_id)
            if not updated:
                raise ResourceNotFoundError("Session", session_id)
            return {"success": True, "data": {"status": "OK", "session_id": session_id}}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def session_end(
        self,
        session_id: str,
        next_action: str | None = None,
        files_modified_count: int = 0,
        tests_run_count: int = 0,
        commits_count: int = 0,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "session:end")
            sess = await proj_ctx.store.get_session(session_id)
            if not sess:
                raise ResourceNotFoundError("Session", session_id)

            pending_tasks = await proj_ctx.bridge.list_tasks(status="pending")
            has_pending = len(pending_tasks) > 0

            # Rule 3 validation
            validate_clean_session_exit(
                session_id=session_id,
                active_task_uuid=sess.get("active_task_uuid"),
                has_pending_tasks=has_pending,
                next_action=next_action,
            )

            closed = await proj_ctx.store.close_session(
                session_id=session_id,
                next_action=next_action,
                files_modified_count=files_modified_count,
                tests_run_count=tests_run_count,
                commits_count=commits_count,
            )

            await proj_ctx.store.record_audit_event(
                project_id=sec_ctx.project,
                session_id=session_id,
                agent_id=sec_ctx.agent_id,
                tool_name="session_end",
                action="END",
                target_resource=session_id,
                success=True,
                execution_duration_ms=5,
            )
            return {"success": True, "data": closed}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_get_next(
        self, component: str | None = None, auth_token: str | None = None
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:read")

            # Rule 7: Check reflection threshold
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            if sess:
                completed_count = sess.get("tasks_completed_count", 0)
                validate_reflection_threshold(completed_count, self.config.reflection_threshold)

            tasks = await proj_ctx.bridge.list_tasks()
            active_blockers = await proj_ctx.store.list_active_blockers(sec_ctx.project)
            blocker_uuids = {b["task_uuid"] for b in active_blockers}

            candidate = select_next_executable_task(
                tasks=tasks,
                active_blocker_uuids=blocker_uuids,
                component=component,
            )
            return {"success": True, "data": candidate}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_start(self, task_uuid: str, auth_token: str | None = None) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:write")
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            if not sess:
                raise ResourceNotFoundError("ActiveSession", sec_ctx.project)

            task = await proj_ctx.bridge.get_task(task_uuid)
            if not task:
                raise ResourceNotFoundError("Task", task_uuid)

            # Rule 6: Validate dependencies completed
            all_tasks = await proj_ctx.bridge.list_tasks()
            tasks_map = {t["uuid"]: t for t in all_tasks}
            validate_dependencies_resolved(task_uuid, task.get("depends", []), tasks_map)

            # Update session's active task
            await proj_ctx.store.update_session(sess["session_id"], active_task_uuid=task_uuid)
            updated = await proj_ctx.bridge.modify_task(
                task_uuid,
                owner=sec_ctx.agent_id,
                session_id=sess["session_id"],
            )
            return {"success": True, "data": updated}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_create(
        self,
        title: str,
        component: str,
        description: str = "",
        priority: str = "M",
        dependencies: list[str] | None = None,
        estimate_minutes: int | None = None,
        confidence: float = 1.0,
        tags: list[str] | None = None,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:write")

            # Check DAG cycles if dependencies given
            if dependencies:
                all_tasks = await proj_ctx.bridge.list_tasks()
                graph = {t["uuid"]: t.get("depends", []) for t in all_tasks}
                temp_uuid = str(uuid.uuid4())
                for dep in dependencies:
                    validate_dependency_acyclic(temp_uuid, dep, graph)

            created = await proj_ctx.bridge.add_task(
                title=title,
                component=component,
                priority=priority,
                tags=tags,
                estimate_minutes=estimate_minutes,
                confidence=confidence,
                owner=sec_ctx.agent_id,
                dependencies=dependencies,
            )
            return {"success": True, "data": created}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_create_subtask(
        self,
        parent_task_uuid: str,
        title: str,
        description: str = "",
        estimate_minutes: int | None = None,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:write")
            parent = await proj_ctx.bridge.get_task(parent_task_uuid)
            if not parent:
                raise ResourceNotFoundError("Task", parent_task_uuid)

            child = await proj_ctx.bridge.add_task(
                title=title,
                component=parent.get("component", "general"),
                priority=parent.get("priority", "M"),
                estimate_minutes=estimate_minutes,
                owner=sec_ctx.agent_id,
            )
            await proj_ctx.bridge.add_dependency(parent_task_uuid, child["uuid"])
            return {"success": True, "data": child}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_complete(
        self,
        task_uuid: str,
        summary: str,
        evidence: str,
        confidence: float = 1.0,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:write")
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            active_task_uuid = sess.get("active_task_uuid") if sess else None

            # Rule 1 & Rule 2 validations
            validate_task_active_for_modification(active_task_uuid, task_uuid)
            validate_completion_evidence(task_uuid, summary, evidence, confidence)

            # Mark task done in Taskwarrior and record evidence
            await proj_ctx.bridge.modify_task(task_uuid, evidence=evidence, confidence=confidence)
            await proj_ctx.bridge.annotate_task(task_uuid, f"COMPLETION SUMMARY: {summary}")
            completed = await proj_ctx.bridge.complete_task(task_uuid)

            # Unset active task in session and increment counter
            if sess:
                await proj_ctx.store.update_session(
                    sess["session_id"],
                    active_task_uuid=None,
                    tasks_completed_count=sess.get("tasks_completed_count", 0) + 1,
                )

            return {"success": True, "data": completed}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_annotate(
        self, task_uuid: str, summary: str, auth_token: str | None = None
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:write")
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            active_task_uuid = sess.get("active_task_uuid") if sess else None

            validate_task_active_for_modification(active_task_uuid, task_uuid)
            annotated = await proj_ctx.bridge.annotate_task(task_uuid, summary)
            return {"success": True, "data": annotated}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_reprioritize(
        self, task_uuid: str, priority: str, auth_token: str | None = None
    ) -> dict[str, Any]:
        try:
            _, proj_ctx = await self._authenticate(auth_token, "task:write")
            updated = await proj_ctx.bridge.modify_task(task_uuid, priority=priority)
            return {"success": True, "data": updated}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_get(self, task_uuid: str, auth_token: str | None = None) -> dict[str, Any]:
        try:
            _, proj_ctx = await self._authenticate(auth_token, "task:read")
            task = await proj_ctx.bridge.get_task(task_uuid)
            if not task:
                raise ResourceNotFoundError("Task", task_uuid)
            return {"success": True, "data": task}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def task_list(
        self,
        status: str | None = None,
        component: str | None = None,
        tags: list[str] | None = None,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            _, proj_ctx = await self._authenticate(auth_token, "task:read")
            tasks = await proj_ctx.bridge.list_tasks(status=status, component=component, tags=tags)
            return {"success": True, "data": tasks}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def observation_record(
        self,
        topic: str,
        content: str,
        component: str,
        confidence: float = 1.0,
        code_references: list[str] | None = None,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "obs:write")
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            session_id = sess["session_id"] if sess else "unbound"

            obs_id = str(uuid.uuid4())
            await proj_ctx.store.record_observation(
                observation_id=obs_id,
                project_id=sec_ctx.project,
                session_id=session_id,
                author_agent=sec_ctx.agent_id,
                topic=topic,
                content=content,
                confidence=confidence,
                component=component,
                code_references=code_references,
            )
            return {"success": True, "data": {"observation_id": obs_id, "status": "STORED"}}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def observation_search(
        self,
        query: str,
        component: str | None = None,
        min_confidence: float = 0.0,
        limit: int = 10,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "obs:read")
            results = await proj_ctx.store.search_observations(
                project_id=sec_ctx.project,
                query=query,
                component=component,
                min_confidence=min_confidence,
                limit=limit,
            )
            return {"success": True, "data": results}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def blocker_create(
        self,
        task_uuid: str,
        reason: str,
        severity: str = "HIGH",
        blocking_task_uuid: str | None = None,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "blocker:manage")
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            session_id = sess["session_id"] if sess else "unbound"

            task = await proj_ctx.bridge.get_task(task_uuid)
            if not task:
                raise ResourceNotFoundError("Task", task_uuid)

            blocker_id = str(uuid.uuid4())
            await proj_ctx.store.create_blocker(
                blocker_id=blocker_id,
                project_id=sec_ctx.project,
                session_id=session_id,
                task_uuid=task_uuid,
                blocking_task_uuid=blocking_task_uuid,
                reason=reason,
                severity=severity,
            )
            return {
                "success": True,
                "data": {"blocker_id": blocker_id, "task_uuid": task_uuid, "status": "BLOCKED"},
            }
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def blocker_resolve(
        self,
        blocker_id: str,
        resolution_summary: str,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            _, proj_ctx = await self._authenticate(auth_token, "blocker:manage")
            resolved = await proj_ctx.store.resolve_blocker(blocker_id, resolution_summary)
            return {"success": True, "data": resolved}
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def workflow_reflect(
        self,
        summary: str,
        stale_tasks_identified: list[str] | None = None,
        recommendations: list[str] | None = None,
        auth_token: str | None = None,
    ) -> dict[str, Any]:
        try:
            sec_ctx, proj_ctx = await self._authenticate(auth_token, "task:write")
            sess = await proj_ctx.store.get_active_session(sec_ctx.project)
            session_id = sess["session_id"] if sess else "unbound"
            completed_count = sess.get("tasks_completed_count", 0) if sess else 0

            ref_id = str(uuid.uuid4())
            await proj_ctx.store.record_reflection(
                reflection_id=ref_id,
                project_id=sec_ctx.project,
                session_id=session_id,
                agent_id=sec_ctx.agent_id,
                completed_tasks_count=completed_count,
                summary=summary,
                stale_tasks_identified=stale_tasks_identified,
                recommendations=recommendations,
            )

            # Reset session completed count counter after reflection
            if sess:
                await proj_ctx.store.update_session(sess["session_id"], tasks_completed_count=0)

            return {
                "success": True,
                "data": {
                    "reflection_id": ref_id,
                    "tasks_completed_since_last_reflection": completed_count,
                    "reflection_gate_cleared": True,
                },
            }
        except TaskManagerError as err:
            return {"success": False, "error": to_mcp_error_data(err)}

    async def run_stdio(self) -> None:
        """Starts the server listening on standard input/output."""
        logger.info("starting_taskmanager_mcp_stdio_server")
        await self.mcp_server.run_stdio_async()
