# Autonomous Agent Task Manager & JWT Auth MCP Architecture

This repository contains two production-ready, professional technical specifications for building an enterprise-grade execution memory and governance system for autonomous coding agents, operating strictly over **Model Context Protocol (MCP) stdio transport**:

1. [**TaskManager MCP Server Specification (`taskmanager-mcp-server.md`)**](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/taskmanager-mcp-server.md):
   * An authoritative execution state manager that enforces disciplined software engineering workflows (e.g. no code edits without an active task, evidence-backed completions, reflection gates, blocker graphs).
   * Backed by isolated Taskwarrior databases for scheduling/dependencies and SQLite for durable observations (with FTS5 full-text search), sessions, and audit logging.
   * Built specifically for standard input/output (`stdio`) transport with multi-tenant project sandboxing.

2. [**JWT Auth MCP Server Specification (`jwt-auth-mcp-server.md`)**](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/jwt-auth-mcp-server.md):
   * A dedicated, lightweight authentication and token lifecycle manager operating over `stdio` transport.
   * Built on **asymmetric cryptography (Ed25519)**: the Auth Manager holds the private signing key, while target MCP servers only require the public key.
   * Features a developer-friendly CLI (`jwt-auth-mcp mint`, `init`, `inspect`, `verify`, `revoke`) that outputs ready-to-paste Claude Desktop and Cursor JSON configuration snippets.
   * Exposes stdio MCP tools (`auth_mint_token`, `auth_verify_token`, `auth_revoke_token`) for supervisor agents and autonomous harnesses.

---

## Architecture Overview

```
                      Developer / Operator
                               │
                 $ jwt-auth-mcp mint --target taskmanager-mcp
                               │
                               ▼
                Generates Scoped JWT Access Token
              (Audience-bound, Ed25519 asymmetric)
                               │
                               ▼
                 Agent Host (Claude / Cursor)
                               │
           Spawns TaskManager MCP Process over stdio
               with TASKMANAGER_AUTH_TOKEN in env
                               │
                               ▼
               TaskManager MCP Server (stdio)
                ├── Security Guard (Verifies JWT via Public Key)
                ├── Workflow Invariant Engine (Rules 1-7)
                ├── Taskwarrior CLI Bridge (Isolated taskrc & data)
                └── SQLite Engine (Observations FTS5 & Sessions)
```

---

## Document Index

* 📘 [TaskManager MCP Server: Technical Specification (v2.0)](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/taskmanager-mcp-server.md)
* 🔐 [JWT Auth MCP Server: Technical Specification (v1.0)](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/jwt-auth-mcp-server.md)
* 🗺️ [Implementation Roadmap (`ROADMAP.md`)](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/ROADMAP.md)
* ⏱️ [Implementation Progress Tracker (`PROGRESS.md`)](file:///Users/sebastientraber/Documents/Prog/sandbox/taskmanager/repo/PROGRESS.md)
