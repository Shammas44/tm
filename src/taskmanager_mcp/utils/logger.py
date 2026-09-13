"""Stderr-only structured JSON logger for TaskManager MCP.

In an MCP stdio server, standard output (stdout) is strictly dedicated to
newline-delimited JSON-RPC 2.0 messages. Any unexpected stdout writes will corrupt
the protocol stream. This module guarantees that all log records and diagnostics
are sent exclusively to standard error (stderr).
"""

import logging
import sys

import structlog


def configure_logging(level: str = "INFO") -> None:
    """Configures structlog to output structured JSON exclusively to stderr."""
    log_level = getattr(logging, level.upper(), logging.INFO)

    # Configure root logging to write to stderr only
    logging.basicConfig(
        format="%(message)s",
        stream=sys.stderr,
        level=log_level,
        force=True,
    )

    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso", utc=True),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.make_filtering_bound_logger(log_level),
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(file=sys.stderr),
        cache_logger_on_first_use=False,
    )


def get_logger(name: str | None = None) -> structlog.BoundLogger:
    """Returns a structured logger instance bound to stderr."""
    logger = structlog.get_logger(name or "taskmanager_mcp")
    return logger
