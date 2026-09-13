import os
import sys
from pathlib import Path

import pytest
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

from taskmanager_mcp.security.jwt_validator import create_test_keypair, create_test_token


@pytest.mark.asyncio
async def test_e2e_stdio_subprocess_and_context_reset(tmp_path: Path):
    """End-to-end integration test executing TaskManager MCP as a real subprocess over stdio,

    including context reset recovery across multiple sessions.
    """
    projects_root = tmp_path / "projects"
    projects_root.mkdir()
    keys_dir = tmp_path / "keys"
    keys_dir.mkdir()

    private_pem, public_pem = create_test_keypair()
    pub_path = keys_dir / "public.pem"
    pub_path.write_text(public_pem)

    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-devin-01",
        project="kernel-e2e",
        role="worker",
    )

    venv_python = sys.executable
    server_params = StdioServerParameters(
        command=str(venv_python),
        args=[
            "-m",
            "taskmanager_mcp",
            "--projects-root",
            str(projects_root),
            "--public-key-path",
            str(pub_path),
            "--log-level",
            "DEBUG",
        ],
        env={
            "TASKMANAGER_AUTH_TOKEN": token,
            "PATH": os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin"),
        },
    )

    async with stdio_client(server_params) as (read, write):
        async with ClientSession(read, write) as session:
            init_res = await session.initialize()
            assert init_res is not None

            # 1. Verify list_tools returns all tools
            tools_list = await session.list_tools()
            tool_names = {t.name for t in tools_list.tools}
            expected_tools = {
                "session_begin",
                "session_heartbeat",
                "session_end",
                "task_get_next",
                "task_start",
                "task_create",
                "task_create_subtask",
                "task_complete",
                "task_annotate",
                "task_reprioritize",
                "task_get",
                "task_list",
                "observation_record",
                "observation_search",
                "blocker_create",
                "blocker_resolve",
                "workflow_reflect",
            }
            assert expected_tools.issubset(tool_names)

            # 2. session_begin via stdio (Session 1)
            sess_call = await session.call_tool(
                "session_begin",
                {"project": "kernel-e2e", "agent_id": "agent-devin-01"},
            )
            assert sess_call.structured_content["success"] is True
            session_id = sess_call.structured_content["data"]["session_id"]
            assert session_id is not None

            # 3. task_create
            create_call = await session.call_tool(
                "task_create",
                {"title": "Implement PCI device enumeration", "component": "pci", "priority": "H"},
            )
            assert create_call.structured_content["success"] is True
            task_uuid = create_call.structured_content["data"]["uuid"]

            # 4. task_start
            start_call = await session.call_tool("task_start", {"task_uuid": task_uuid})
            assert start_call.structured_content["success"] is True

            # 5. observation_record
            obs_call = await session.call_tool(
                "observation_record",
                {
                    "topic": "PCI BAR Sizing",
                    "content": "Writing all 1s to BAR register determines address space requirements.",
                    "component": "pci",
                    "confidence": 0.96,
                },
            )
            assert obs_call.structured_content["success"] is True

            # 6. task_complete (evidence-backed)
            comp_call = await session.call_tool(
                "task_complete",
                {
                    "task_uuid": task_uuid,
                    "summary": "Completed PCI device enumeration and BAR allocation successfully.",
                    "evidence": "qemu kernel boot log verified with 4 devices detected.",
                    "confidence": 0.99,
                },
            )
            assert comp_call.structured_content["success"] is True

            # 7. session_end (Session 1 closes with next_action)
            end_call = await session.call_tool(
                "session_end",
                {
                    "session_id": session_id,
                    "next_action": "Implement MSI interrupt configuration.",
                },
            )
            assert end_call.structured_content["success"] is True
            assert end_call.structured_content["data"]["status"] == "CLOSED"

            # 8. CONTEXT RESET SIMULATION (Session 2 begins)
            sess2_call = await session.call_tool(
                "session_begin",
                {"project": "kernel-e2e", "agent_id": "agent-devin-01"},
            )
            assert sess2_call.structured_content["success"] is True
            sess2_data = sess2_call.structured_content["data"]
            assert sess2_data["session_id"] != session_id

            # 9. Verify prior session's observations are retrievable across sessions
            obs_search = await session.call_tool(
                "observation_search",
                {"query": "BAR register address space"},
            )
            assert obs_search.structured_content["success"] is True
            assert len(obs_search.structured_content["data"]) >= 1
            assert obs_search.structured_content["data"][0]["topic"] == "PCI BAR Sizing"

            # 10. Close Session 2
            await session.call_tool(
                "session_end",
                {"session_id": sess2_data["session_id"], "next_action": "Done."},
            )
