from pathlib import Path

import pytest

from taskmanager_mcp.errors import (
    AuthenticationError,
    ForbiddenPermissionError,
    ProjectMismatchError,
)
from taskmanager_mcp.security.jwt_validator import (
    JWTValidator,
    SecurityContext,
    create_test_keypair,
    create_test_token,
)
from taskmanager_mcp.security.project_guard import resolve_project_dir


@pytest.fixture
def keys(tmp_path: Path):
    private_pem, public_pem = create_test_keypair()
    pub_file = tmp_path / "public.pem"
    pub_file.write_text(public_pem)
    return private_pem, public_pem, pub_file


def test_jwt_validation_success(keys):
    private_pem, _, pub_file = keys
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-claude-01",
        project="compiler",
        role="worker",
        permissions=["task:*", "session:*", "obs:*"],
        ttl_minutes=60,
    )

    validator = JWTValidator(public_key_path=pub_file, expected_aud="taskmanager-mcp")
    ctx = validator.verify_token(token)

    assert isinstance(ctx, SecurityContext)
    assert ctx.agent_id == "agent-claude-01"
    assert ctx.project == "compiler"
    assert ctx.role == "worker"
    assert ctx.has_permission("task:start") is True
    assert ctx.has_permission("task:complete") is True
    assert ctx.has_permission("admin:purge") is False


def test_jwt_validation_expired(keys):
    private_pem, _, pub_file = keys
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-01",
        project="compiler",
        ttl_minutes=-10,  # Expired
    )

    validator = JWTValidator(public_key_path=pub_file)
    with pytest.raises(AuthenticationError) as exc_info:
        validator.verify_token(token)
    assert "expired" in str(exc_info.value).lower() or "signature" in str(exc_info.value).lower()


def test_jwt_validation_wrong_audience(keys):
    private_pem, _, pub_file = keys
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-01",
        project="compiler",
        aud="other-mcp",
    )

    validator = JWTValidator(public_key_path=pub_file, expected_aud="taskmanager-mcp")
    with pytest.raises(AuthenticationError):
        validator.verify_token(token)


def test_jwt_rbac_permission_enforcement(keys):
    private_pem, _, pub_file = keys
    token = create_test_token(
        private_key_pem=private_pem,
        sub="agent-observer",
        project="compiler",
        role="observer",
        permissions=["task:read", "obs:read"],
    )

    validator = JWTValidator(public_key_path=pub_file)
    ctx = validator.verify_token(token)

    # Allowed
    ctx.enforce_permission("task:read")

    # Forbidden
    with pytest.raises(ForbiddenPermissionError):
        ctx.enforce_permission("task:complete")


def test_project_guard_and_confinement(tmp_path: Path):
    projects_root = tmp_path / "projects"
    projects_root.mkdir()

    # Valid project
    proj_dir = resolve_project_dir(projects_root, "compiler")
    assert proj_dir == projects_root / "compiler"

    # Path traversal attack attempt
    with pytest.raises(ProjectMismatchError):
        resolve_project_dir(projects_root, "../etc/passwd")

    # Invalid project slug
    with pytest.raises(ProjectMismatchError):
        resolve_project_dir(projects_root, "project$invalid!")
