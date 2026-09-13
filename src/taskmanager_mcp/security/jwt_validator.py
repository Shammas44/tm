"""JWT validation and Role-Based Access Control (RBAC) security context."""

import uuid
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

import jwt
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ed25519

from taskmanager_mcp.errors import AuthenticationError, ForbiddenPermissionError
from taskmanager_mcp.utils.logger import get_logger

logger = get_logger("jwt_validator")


def create_test_keypair() -> tuple[str, str]:
    """Generates an Ed25519 private and public keypair in PEM format for testing."""
    private_key = ed25519.Ed25519PrivateKey.generate()
    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")

    public_key = private_key.public_key()
    public_pem = public_key.public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    ).decode("utf-8")

    return private_pem, public_pem


def create_test_token(
    private_key_pem: str,
    sub: str,
    project: str,
    role: str = "worker",
    permissions: list[str] | None = None,
    ttl_minutes: int = 60,
    aud: str = "taskmanager-mcp",
    iss: str = "mcp-jwt-auth-manager",
) -> str:
    """Mints a signed JWT access token for testing using Ed25519."""
    now = datetime.now(UTC)
    exp = now + timedelta(minutes=ttl_minutes)

    if permissions is None:
        if role == "admin":
            permissions = ["*"]
        elif role == "worker":
            permissions = ["task:*", "session:*", "obs:*", "blocker:*"]
        elif role == "reviewer":
            permissions = ["task:read", "task:approve", "session:*", "obs:read", "audit:read"]
        else:
            permissions = ["task:read", "obs:read"]

    payload = {
        "iss": iss,
        "sub": sub,
        "aud": aud,
        "project": project,
        "role": role,
        "permissions": permissions,
        "iat": int(now.timestamp()),
        "exp": int(exp.timestamp()),
        "jti": str(uuid.uuid4()),
    }

    token = jwt.encode(payload, private_key_pem, algorithm="EdDSA")
    return token


class SecurityContext:
    """Represents the authenticated agent identity and permissions."""

    def __init__(
        self,
        agent_id: str,
        project: str,
        role: str,
        permissions: list[str],
        token_id: str,
        expires_at: int,
    ):
        self.agent_id = agent_id
        self.project = project
        self.role = role
        self.permissions = permissions
        self.token_id = token_id
        self.expires_at = expires_at

    def has_permission(self, required_permission: str) -> bool:
        """Evaluates whether the context holds the required permission (supporting wildcards)."""
        if "*" in self.permissions:
            return True

        if required_permission in self.permissions:
            return True

        # Check prefix wildcard (e.g. 'task:*' satisfies 'task:read')
        if ":" in required_permission:
            namespace = required_permission.split(":")[0]
            if f"{namespace}:*" in self.permissions:
                return True

        return False

    def enforce_permission(self, required_permission: str) -> None:
        """Raises ForbiddenPermissionError if the permission is not granted."""
        if not self.has_permission(required_permission):
            raise ForbiddenPermissionError(
                permission=required_permission,
                detail=f"Agent '{self.agent_id}' lacks permission '{required_permission}'.",
            )


class JWTValidator:
    """Validates JWT access tokens against public keys or shared secrets."""

    def __init__(
        self,
        public_key_path: Path | str | None = None,
        public_key_pem: str | None = None,
        secret: str | None = None,
        expected_aud: str = "taskmanager-mcp",
        expected_iss: str | None = None,
    ):
        self.expected_aud = expected_aud
        self.expected_iss = expected_iss
        self.secret = secret

        self.verification_key: Any = None
        if public_key_pem:
            self.verification_key = public_key_pem
        elif public_key_path:
            p = Path(public_key_path)
            if p.exists():
                self.verification_key = p.read_text(encoding="utf-8")

        if not self.verification_key and not self.secret:
            logger.warning("jwt_validator_unconfigured_keys")

    def verify_token(self, token: str) -> SecurityContext:
        """Cryptographically verifies the token and returns an authenticated SecurityContext."""
        if not token:
            raise AuthenticationError("No token provided.")

        key = self.verification_key or self.secret
        if not key:
            raise AuthenticationError("Validator has no public key or secret configured.")

        # Determine algorithms to accept
        algorithms = ["EdDSA", "RS256", "HS256"] if not self.secret else ["HS256"]

        options = {
            "require": ["exp", "sub", "project"],
            "verify_aud": self.expected_aud is not None,
        }

        try:
            payload = jwt.decode(
                token,
                key,
                algorithms=algorithms,
                audience=self.expected_aud,
                issuer=self.expected_iss,
                options=options,
            )
        except jwt.ExpiredSignatureError:
            raise AuthenticationError("Token has expired.") from None
        except jwt.InvalidAudienceError:
            raise AuthenticationError(
                f"Invalid token audience. Expected: '{self.expected_aud}'."
            ) from None
        except jwt.PyJWTError as err:
            raise AuthenticationError(
                f"Token signature or claims verification failed: {err}"
            ) from None

        agent_id = payload.get("sub")
        project = payload.get("project")
        role = payload.get("role", "worker")
        permissions = payload.get("permissions", [])
        token_id = payload.get("jti", "")
        expires_at = payload.get("exp", 0)

        if not agent_id or not project:
            raise AuthenticationError("Token missing mandatory 'sub' or 'project' claims.")

        return SecurityContext(
            agent_id=agent_id,
            project=project,
            role=role,
            permissions=permissions,
            token_id=token_id,
            expires_at=expires_at,
        )
