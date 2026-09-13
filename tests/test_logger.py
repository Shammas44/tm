import io
import json
import sys

from taskmanager_mcp.utils.logger import configure_logging, get_logger


def test_logger_writes_strictly_to_stderr(monkeypatch):
    stderr_capture = io.StringIO()
    stdout_capture = io.StringIO()
    monkeypatch.setattr(sys, "stderr", stderr_capture)
    monkeypatch.setattr(sys, "stdout", stdout_capture)

    configure_logging(level="DEBUG")
    logger = get_logger("test")
    logger.info("tool_invoked", tool="task_start", agent_id="agent-01", project="compiler")

    # stdout MUST be completely clean for stdio JSON-RPC transport!
    assert stdout_capture.getvalue() == ""

    # stderr contains JSON formatted log line
    stderr_output = stderr_capture.getvalue()
    assert stderr_output != ""

    parsed = json.loads(stderr_output.strip().split("\n")[-1])
    assert parsed["event"] == "tool_invoked"
    assert parsed["tool"] == "task_start"
    assert parsed["agent_id"] == "agent-01"
    assert parsed["project"] == "compiler"
    assert "timestamp" in parsed
