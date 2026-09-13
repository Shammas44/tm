# JWT Auth MCP Server: Technical Specification (v1.0)
## Dedicated Token Lifecycle & Authentication Manager over Stdio Transport

---

## 1. Executive Summary & Purpose

The **JWT Auth MCP Server** is a specialized, lightweight, security-first service operating exclusively over **standard input/output (`stdio`) transport**. It solves the problem of decentralized, fragile, or hardcoded authentication tokens for autonomous AI agents in Model Context Protocol (MCP) ecosystems.

### 1.1 Core Problems Solved
* **Missing Transport Authentication:** Standard MCP over `stdio` lacks HTTP headers (such as `Authorization: Bearer <token>`). Authentication tokens must be managed, minted, and verified through standardized channels without breaking JSON-RPC framing.
* **Privilege Escalation Risks:** Symmetric tokens (e.g., HS256) require every target MCP server to share the secret key. If a worker MCP server is compromised, it can forge tokens for all other servers. The JWT Auth MCP uses **asymmetric cryptography (Ed25519)**: only the Auth Manager holds the private signing key; target MCP servers only receive the public key.
* **Developer Friction:** Developers configuring AI agents in Claude Desktop, Cursor, or autonomous scripts find generating cryptographic tokens tedious. The JWT Auth Manager provides a **developer-friendly CLI with ready-to-copy configuration templates** alongside its MCP interface.
* **Token Creep & Revocation:** Provides an auditable lifecycle with explicit expiration (`exp`), audience-binding (`aud`), and instant token revocation (`jti` denylist).

### 1.2 Dual Interface Design
```
                       ┌─────────────────────────────────────────┐
                       │           JWT Auth Manager Core         │
                       │   (Ed25519 Keystore & SQLite Denylist)  │
                       └────────────────────┬────────────────────┘
                                            │
                    ┌───────────────────────┴───────────────────────┐
                    ▼                                               ▼
         Developer Terminal / CLI                        MCP Server (Stdio)
       $ jwt-auth-mcp mint ...                         JSON-RPC 2.0 via stdin/stdout
                    │                                               │
      • Interactive token wizard                      • Invoked by Agent Orchestrators
      • Claude Desktop JSON output                    • `auth_mint_token`
      • Token inspection & verification               • `auth_verify_token`
      • Emergency revocation                          • `auth_revoke_token`
```

---

## 2. Cryptographic & Key Architecture

### 2.1 Cryptographic Standard: EdDSA (Ed25519)
The server uses **Ed25519 (Edwards-curve Digital Signature Algorithm)** as its primary signing algorithm:
* **High Security & Speed:** State-of-the-art security (128-bit security level) with microsecond signing and verification latency.
* **Compact Footprint:** Public keys are 32 bytes; signatures are 64 bytes. Resulting JWTs are significantly shorter than RSA tokens, reducing prompt context overhead when passed across tools.
* **Fallback Algorithm:** Supports **RS256 (RSA-2048/4096)** and **HS256 (HMAC-SHA256)** via configuration flag for legacy environments.

### 2.2 Asymmetric Key Isolation Principle
```
┌──────────────────────────────────────┐          ┌──────────────────────────────────────┐
│       JWT Auth Manager (This MCP)    │          │      Target MCP (e.g. TaskManager)   │
│                                      │          │                                      │
│  Private Key (private_key.pem) 0600  │          │  Public Key (public_key.pem) 0644    │
│  • Used exclusively to SIGN tokens   │          │  • Used ONLY to VERIFY tokens        │
│  • NEVER shared with target MCPs     │          │  • CANNOT forge or mint tokens       │
└──────────────────────────────────────┘          └──────────────────────────────────────┘
```

### 2.3 Keystore Directory Hierarchy
The keystore resides in the user's home directory with strict POSIX permissions (`0700` for directory, `0600` for private keys):

```
~/.mcp-auth/
├── keys/
│   ├── active_kid.txt               # ID of current active signing key (e.g. "key-2026-09")
│   ├── key-2026-09.private.pem      # Private key (chmod 0600)
│   ├── key-2026-09.public.pem       # Public key (chmod 0644)
│   └── jwks.json                    # Exported JSON Web Key Set of all valid public keys
├── revocations.db                   # SQLite database of revoked token JTIs
└── config.yaml                      # Global defaults (issuer, default TTL, algorithms)
```

---

## 3. Standardized Token Structure & Claims Specification

Every minted token conforms to RFC 7519 (JWT) and RFC 7515 (JWS).

### 3.1 JWT Header
```json
{
  "alg": "EdDSA",
  "typ": "JWT",
  "kid": "key-2026-09"
}
```

### 3.2 JWT Payload Claims
```json
{
  "iss": "mcp-jwt-auth-manager",
  "sub": "agent-claude-worker-1",
  "aud": "taskmanager-mcp",
  "project": "compiler-core",
  "role": "worker",
  "permissions": [
    "session:start",
    "session:end",
    "task:read",
    "task:write",
    "task:complete",
    "obs:read",
    "obs:write",
    "blocker:manage"
  ],
  "session_max_duration_min": 240,
  "iat": 1773489600,
  "nbf": 1773489600,
  "exp": 1773518400,
  "jti": "7f8b9d4e-12ab-4c3d-8e9f-0123456789ab"
}
```

### 3.3 Claims Reference Table

| Claim | Key | Type | Description | Mandatory |
|---|---|---|---|:---:|
| **Issuer** | `iss` | string | Canonical identifier of this auth server (e.g. `mcp-jwt-auth-manager`). | Yes |
| **Subject** | `sub` | string | Unique agent identity (e.g. `agent-claude`, `devin-worker-3`). | Yes |
| **Audience** | `aud` | string | Targeted MCP server identifier (e.g. `taskmanager-mcp`, `github-mcp`). | Yes |
| **Project** | `project` | string | Project namespace to confine access (e.g. `compiler-core`). | Yes |
| **Role** | `role` | string | Role classification: `admin`, `worker`, `reviewer`, `observer`. | Yes |
| **Permissions**| `permissions`| list[str] | Granular capability strings or wildcard expressions. | Yes |
| **Issued At** | `iat` | integer | Epoch seconds when token was generated. | Yes |
| **Not Before** | `nbf` | integer | Epoch seconds before which token is invalid (defaults to `iat`). | Yes |
| **Expiration** | `exp` | integer | Epoch seconds when token becomes invalid. | Yes |
| **Token ID** | `jti` | string | Cryptographically secure UUIDv4 for tracking and revocation. | Yes |
| **Duration Limit**| `session_max_duration_min` | integer | Optional session length limit enforced by target MCP. | No |

---

## 4. Token Revocation Engine

To prevent orphaned or compromised agents from executing tasks after their credentials should be invalidated, the manager implements an SQLite-backed revocation registry.

### 4.1 Revocation Database Schema (`revocations.db`)
```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS revoked_tokens (
    jti TEXT PRIMARY KEY,
    sub TEXT NOT NULL,
    aud TEXT NOT NULL,
    project TEXT NOT NULL,
    revoked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    reason TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_revoked_expiry ON revoked_tokens(expires_at);

-- Automatic cleanup trigger for expired tokens:
-- Expired entries are safely purged during write operations because their natural `exp` already invalidates them.
```

### 4.2 Revocation Verification Workflow
1. When target MCPs verify a token, they can optionally consult the shared `revocations.db` (opened in read-only URI mode `file:revocations.db?mode=ro`) or query the auth manager's `auth_verify_token` tool.
2. If `jti` is found in `revoked_tokens`, the token is rejected immediately with error `TOKEN_REVOKED`.

---

## 5. Developer CLI: User-Friendly Token Management

The package provides a rich, developer-friendly command-line utility: `jwt-auth-mcp`.

```
$ jwt-auth-mcp --help
Usage: jwt-auth-mcp [OPTIONS] COMMAND [ARGS]...

  Dedicated JWT Authentication Manager for Stdio Model Context Protocol.

Commands:
  init          Initialize keystore and generate master Ed25519 keys.
  mint          Generate a scoped JWT access token for a target MCP server.
  inspect       Decode, pretty-print, and inspect claims of a token.
  verify        Validate signature, expiration, and revocation of a token.
  revoke        Revoke an active token by JTI or token string.
  keys          Manage signing keys (list, rotate, export public JWKS).
```

### 5.1 Initial Setup (`init`)
Generates initial directory structure, creates master Ed25519 keypair, and sets strict permissions:
```bash
$ jwt-auth-mcp init
✔ Created keystore directory: ~/.mcp-auth/keys
✔ Generated Ed25519 keypair: key-2026-09 (Active)
✔ Initialized revocation store: ~/.mcp-auth/revocations.db
✔ Keystore initialized successfully with permissions 0700/0600.
```

### 5.2 Minting Tokens for Developers (`mint`)

#### Interactive Mode
If run without arguments, the CLI opens an interactive terminal wizard:
```bash
$ jwt-auth-mcp mint
? Target MCP Server (aud): taskmanager-mcp
? Agent Identity (sub): agent-claude-worker
? Target Project: compiler-core
? Agent Role: worker
? Select Permissions: [X] task:read [X] task:write [X] session:* [X] obs:*
? Token Lifetime (e.g. 1h, 8h, 7d): 8h
? Output Format: Claude Desktop Configuration Snippet
```

#### Non-Interactive One-Liner
```bash
# Mint token for TaskManager MCP with 8 hour validity
$ jwt-auth-mcp mint \
    --target taskmanager-mcp \
    --agent agent-claude-01 \
    --project compiler-core \
    --role worker \
    --ttl 8h \
    --format claude-config
```

#### Output: Ready-to-Paste Configuration Snippet
When `--format claude-config` is selected, the CLI outputs a fully populated JSON block ready to paste into `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "taskmanager": {
      "command": "uv",
      "args": [
        "run",
        "--with", "taskmanager-mcp",
        "taskmanager-mcp"
      ],
      "env": {
        "TASKMANAGER_AUTH_TOKEN": "eyJhbGciOiJFZERTQSI...",
        "TASKMANAGER_PUBLIC_KEY_PATH": "/Users/developer/.mcp-auth/keys/key-2026-09.public.pem"
      }
    }
  }
}
```

### 5.3 Inspecting Tokens (`inspect`)
Allows developers to quickly check what a mystery token does without needing external web tools like jwt.io:

```bash
$ jwt-auth-mcp inspect eyJhbGciOiJFZERTQSI...
```
**Terminal Output:**
```
╭────────────────────── JWT Token Inspector ──────────────────────╮
│ Header:                                                        │
│   alg: EdDSA                                                   │
│   typ: JWT                                                     │
│   kid: key-2026-09                                             │
│                                                                │
│ Claims:                                                        │
│   iss: mcp-jwt-auth-manager                                    │
│   sub: agent-claude-01 (Agent Identity)                        │
│   aud: taskmanager-mcp (Target MCP Server)                     │
│   project: compiler-core (Confined Project)                    │
│   role: worker                                                 │
│   permissions: [session:*, task:*, obs:*, blocker:*]           │
│   jti: 7f8b9d4e-12ab-4c3d-8e9f-0123456789ab                   │
│                                                                │
│ Lifetime Status:                                               │
│   Issued At:   2026-09-13 12:00:00 UTC                         │
│   Expires At:  2026-09-13 20:00:00 UTC                         │
│   Status:      VALID (Remaining: 7 hours, 42 minutes)          │
╰────────────────────────────────────────────────────────────────╯
```

### 5.4 Emergency Revocation (`revoke`)
```bash
# Revoke by JTI or by passing raw token
$ jwt-auth-mcp revoke 7f8b9d4e-12ab-4c3d-8e9f-0123456789ab --reason "Agent went out of bounds"
✔ Token 7f8b9d4e-12ab-4c3d-8e9f-0123456789ab successfully revoked.
```

---

## 6. Stdio MCP Tool Specifications

When running in MCP server mode via `stdio`, autonomous supervisor agents or orchestrator harnesses can invoke tools over JSON-RPC 2.0.

### 6.1 `auth_mint_token`
Issues a new cryptographically signed access token.

* **Tool Description:** Mints a new scoped JWT access token for an agent targeting a specific MCP server.
* **Input Schema:**
  ```json
  {
    "type": "object",
    "properties": {
      "target_aud": {
        "type": "string",
        "description": "Identifier of the target MCP server (e.g. 'taskmanager-mcp')."
      },
      "agent_sub": {
        "type": "string",
        "description": "Unique agent identifier (e.g. 'subagent-test-runner-4')."
      },
      "project": {
        "type": "string",
        "description": "Target project sandbox namespace."
      },
      "role": {
        "type": "string",
        "enum": ["admin", "worker", "reviewer", "observer"],
        "default": "worker"
      },
      "permissions": {
        "type": "array",
        "items": {"type": "string"},
        "description": "List of granted capabilities. Wildcards allowed."
      },
      "ttl_minutes": {
        "type": "integer",
        "default": 480,
        "description": "Token validity duration in minutes."
      }
    },
    "required": ["target_aud", "agent_sub", "project", "permissions"]
  }
  ```
* **Output Schema:**
  ```json
  {
    "token": "eyJhbGciOiJFZERTQSI...",
    "jti": "7f8b9d4e-12ab-4c3d-8e9f-0123456789ab",
    "expires_at": 1773518400,
    "target_aud": "taskmanager-mcp"
  }
  ```

### 6.2 `auth_verify_token`
Verifies a token's cryptographic integrity, claims, and revocation status.

* **Input Schema:**
  ```json
  {
    "type": "object",
    "properties": {
      "token": {"type": "string"},
      "expected_aud": {
        "type": "string",
        "description": "Expected audience string (e.g. 'taskmanager-mcp')."
      },
      "required_permissions": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Optional list of permissions required to pass verification."
      }
    },
    "required": ["token"]
  }
  ```
* **Output Schema:**
  ```json
  {
    "valid": true,
    "claims": {
      "sub": "agent-claude-01",
      "aud": "taskmanager-mcp",
      "project": "compiler-core",
      "role": "worker",
      "permissions": ["task:read", "task:write"]
    },
    "expires_in_seconds": 27720
  }
  ```

### 6.3 `auth_revoke_token`
Revokes an active token.

* **Input Schema:**
  ```json
  {
    "type": "object",
    "properties": {
      "jti_or_token": {"type": "string"},
      "reason": {"type": "string", "minLength": 5}
    },
    "required": ["jti_or_token", "reason"]
  }
  ```
* **Output Schema:**
  ```json
  {
    "success": true,
    "revoked_jti": "7f8b9d4e-12ab-4c3d-8e9f-0123456789ab",
    "revoked_at": "2026-09-13T12:05:00Z"
  }
  ```

### 6.4 `auth_get_public_keys`
Retrieves valid public keys in standard PEM and JWKS formats for target MCP configuration.

* **Input Schema:** `{"type": "object", "properties": {}}`
* **Output Schema:**
  ```json
  {
    "active_kid": "key-2026-09",
    "public_keys": [
      {
        "kid": "key-2026-09",
        "alg": "EdDSA",
        "pem": "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEA..."
      }
    ]
  }
  ```

---

## 7. Target MCP Server Integration Library

To make integrating any target MCP server (such as `taskmanager-mcp`) effortless, the package includes a standalone, dependency-minimal verification module: `mcp_auth_verifier`.

### 7.1 Target MCP Verification Code Example
Any target MCP server can enforce authentication with fewer than 15 lines of code:

```python
from mcp_auth_verifier import MCPTokenVerifier, TokenValidationError

# Initialize verifier with public key path or string
verifier = MCPTokenVerifier(
    public_key_path="/etc/mcp/keys/public.pem",
    expected_aud="taskmanager-mcp",
    revocations_db_path="/etc/mcp/revocations.db"  # Optional local check
)

def authenticate_tool_request(token: str, required_permission: str) -> dict:
    try:
        claims = verifier.verify(
            token=token,
            required_permissions=[required_permission]
        )
        return claims
    except TokenValidationError as err:
        raise PermissionError(f"Authentication failed: {err.message} (code: {err.code})")
```

---

## 8. Standardized Error Codes

| Error Code | Description | Remedy |
|---|---|---|
| `TOKEN_EXPIRED` | Current time is past `exp` claim. | Re-mint a new token with updated expiration. |
| `SIGNATURE_INVALID` | Token signature mismatch or signed with untrusted key. | Verify keystore public key matches active signing key. |
| `AUDIENCE_MISMATCH` | Token `aud` does not match target MCP server. | Mint token specifically specifying `--target <mcp-name>`. |
| `PERMISSION_DENIED` | Token lacks one or more `required_permissions`. | Request token with elevated permissions or appropriate role. |
| `TOKEN_REVOKED` | Token's `jti` is registered in the revocation store. | Token has been decommissioned; generate a replacement. |
| `KEY_NOT_FOUND` | Key specified by `kid` header does not exist in keystore. | Rotate or sync public keys with `jwt-auth-mcp keys sync`. |

---

## 9. Technology Stack & Python Package Layout

### 9.1 Core Technology Stack
* **Language:** Python 3.12+
* **MCP Protocol SDK:** `mcp` (Official Anthropic Python SDK)
* **JWT & Cryptography:** `pyjwt[crypto]`, `cryptography`
* **CLI Engine:** `typer` or `click`, `rich` (for formatting, tables, prompts)
* **Local Storage:** `sqlite3` (built-in standard library)
* **Packaging:** `uv`, `pyproject.toml`

### 9.2 Package Architecture
```
jwt_auth_mcp/
├── __init__.py
├── __main__.py                      # CLI entrypoint ($ jwt-auth-mcp)
├── cli/
│   ├── commands_init.py             # Keystore setup
│   ├── commands_mint.py             # Interactive and CLI token minting
│   ├── commands_inspect.py          # Terminal token decode & inspector
│   ├── commands_verify.py           # Verification & revocation CLI commands
│   └── formatters.py                # Rich terminal tables and JSON templates
├── core/
│   ├── keystore.py                  # Ed25519/RSA file-based key manager
│   ├── signer.py                    # Token creation & signing engine
│   ├── verifier.py                  # Cryptographic validation & claims engine
│   └── revocation_store.py          # SQLite denylist CRUD
├── server/
│   ├── stdio_server.py              # MCP Server over stdio
│   └── tools.py                     # auth_* MCP tool handlers
└── verifier_client/
    └── verifier.py                  # Lightweight standalone verifier for target MCPs
```
