"""Main executable entrypoint for TaskManager MCP Server over stdio."""

import argparse
import asyncio
import sys
from pathlib import Path

from taskmanager_mcp.config import ServerConfig
from taskmanager_mcp.server import TaskManagerServer
from taskmanager_mcp.utils.logger import configure_logging, get_logger

logger = get_logger("cli")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="TaskManager MCP Server: High-Assurance Persistent Execution Memory over Stdio Transport"
    )
    parser.add_argument(
        "--projects-root",
        type=Path,
        default=None,
        help="Root directory where isolated project workspaces are stored.",
    )
    parser.add_argument(
        "--public-key-path",
        type=Path,
        default=None,
        help="Path to Ed25519/RSA public key PEM file for JWT validation.",
    )
    parser.add_argument(
        "--log-level",
        type=str,
        default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="Logging level for stderr structured logger.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    configure_logging(level=args.log_level)

    config = ServerConfig()
    if args.projects_root:
        config.projects_root = args.projects_root
    if args.public_key_path:
        config.jwt_public_key_path = args.public_key_path

    logger.info(
        "starting_taskmanager_mcp",
        projects_root=str(config.projects_root),
        public_key_path=str(config.jwt_public_key_path) if config.jwt_public_key_path else None,
    )

    server = TaskManagerServer(config=config)
    try:
        asyncio.run(server.run_stdio())
    except KeyboardInterrupt:
        logger.info("server_stopped_by_user")
        sys.exit(0)
    except Exception as exc:
        logger.error("fatal_server_error", error=str(exc))
        sys.exit(1)


if __name__ == "__main__":
    main()
