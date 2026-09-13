#!/usr/bin/env python3
"""Developer utility to generate keypairs and mint JWT tokens for TaskManager MCP."""

import argparse
import json
from pathlib import Path

from taskmanager_mcp.security.jwt_validator import create_test_keypair, create_test_token


def main():
    parser = argparse.ArgumentParser(
        description="Mint JWT tokens for local TaskManager MCP testing"
    )
    parser.add_argument("--project", default="default-project", help="Target project slug")
    parser.add_argument("--agent", default="agent-claude", help="Agent identity (sub claim)")
    parser.add_argument(
        "--role",
        default="worker",
        choices=["worker", "admin", "reviewer", "observer"],
        help="Agent role",
    )
    parser.add_argument(
        "--ttl", type=int, default=480, help="Token TTL in minutes (default: 480 / 8 hours)"
    )
    parser.add_argument(
        "--keys-dir",
        type=Path,
        default=Path.home() / ".taskmanager" / "keys",
        help="Directory for key storage",
    )
    args = parser.parse_args()

    args.keys_dir.mkdir(parents=True, exist_ok=True)
    priv_file = args.keys_dir / "private.pem"
    pub_file = args.keys_dir / "public.pem"

    if not priv_file.exists() or not pub_file.exists():
        print(f"Generating new Ed25519 keypair in {args.keys_dir}...")
        priv_pem, pub_pem = create_test_keypair()
        priv_file.write_text(priv_pem)
        priv_file.chmod(0o600)
        pub_file.write_text(pub_pem)
        pub_file.chmod(0o644)
    else:
        priv_pem = priv_file.read_text()
        pub_pem = pub_file.read_text()

    token = create_test_token(
        private_key_pem=priv_pem,
        sub=args.agent,
        project=args.project,
        role=args.role,
        ttl_minutes=args.ttl,
    )

    print("\n" + "=" * 60)
    print(
        f"✔ Minted JWT token for agent '{args.agent}' on project '{args.project}' (role: {args.role})"
    )
    print(f"✔ Public key: {pub_file}")
    print("=" * 60)
    print(f"\nRAW TOKEN:\n{token}\n")

    claude_snippet = {
        "mcpServers": {
            "taskmanager": {
                "command": "python3",
                "args": [
                    "-m",
                    "taskmanager_mcp",
                    "--projects-root",
                    str(Path.home() / ".taskmanager" / "projects"),
                    "--public-key-path",
                    str(pub_file),
                ],
                "env": {
                    "TASKMANAGER_AUTH_TOKEN": token,
                },
            }
        }
    }

    print("=" * 60)
    print("CLAUDE DESKTOP CONFIGURATION SNIPPET:")
    print("=" * 60)
    print(json.dumps(claude_snippet, indent=2))
    print("=" * 60 + "\n")


if __name__ == "__main__":
    main()
