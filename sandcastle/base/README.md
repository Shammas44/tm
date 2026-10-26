# Sandcastle Base Images with TaskManager (`tm`)

This directory provides production-ready **base Docker images** that combine **TaskManager (`tm`)** and **Sandcastle** sandbox environments with either **Claude Code** or **Google Antigravity CLI (`agy`)**.

By baking `tm`, system utilities, and AI coding agents directly into the base image:
* Child projects do **not** need to bind-mount the host `tm` binary.
* `tm` is instantly available system-wide at `/usr/local/bin/tm`.
* Any project can simply write `FROM sandcastle-base:claude` or `FROM sandcastle-base:agy` to add its own language toolchains (Python, Go, Rust, Node, etc.).

---

## Included Components

| Component | `sandcastle-base:claude` | `sandcastle-base:agy` | Purpose |
| :--- | :--- | :--- | :--- |
| **TaskManager (`tm`)** | `/usr/local/bin/tm` | `/usr/local/bin/tm` | Embedded task management & state recovery engine |
| **Core Utilities** | `curl`, `jq`, `ca-certificates`, `procps`, `build-essential` | `curl`, `jq`, `ca-certificates`, `procps`, `build-essential` | System inspection, compiling, network calls |
| **Git** | `git` (with `safe.directory '*'`) | `git` (with `safe.directory '*'`) | Worktree support & version control |
| **Sudo** | Passwordless sudo for `agent` | Passwordless sudo for `agent` | Project dependency installations |
| **Editor** | `vim` (`EDITOR=vim`, `VISUAL=vim`) | `vim` (`EDITOR=vim`, `VISUAL=vim`) | Interactive task edits (`tm edit`, TUI editor) |
| **Agent CLI** | Anthropic Claude Code (`@anthropic-ai/claude-code`) + `gh` | Google Antigravity CLI (`agy`) | Autonomous AI coding agent |
| **Runtime Base** | `node:22-bookworm-slim` | `debian:bookworm-slim` | Minimal, stable base distribution |
| **User** | `agent` (mapped to host UID/GID) | `agent` (mapped to host UID/GID) | Safe non-root container workspace |

---

## Building the Base Images

The base images download the pre-compiled, stripped static Linux binary directly from **GitHub Releases** (`https://github.com/Shammas44/tm/releases`). No Go compiler, build tools, git clone, or source files are needed—only the single standalone executable (`/usr/local/bin/tm`) is kept.

From this directory or the project root, run:

```bash
# Build both base images using latest GitHub release
bash sandcastle/base/build-base.sh

# Or build only one target:
bash sandcastle/base/build-base.sh claude
bash sandcastle/base/build-base.sh agy

# Pin to a specific release version (e.g. v0.1.0):
TM_VERSION=v0.1.0 bash sandcastle/base/build-base.sh
```

Alternatively, invoke `docker build` directly:

```bash
# Build Claude Code base image directly from GitHub Release
docker build \
  --tag sandcastle-base:claude \
  --build-arg AGENT_UID="$(id -u)" \
  --build-arg AGENT_GID="$(id -g)" \
  --build-arg TM_VERSION="latest" \
  --file sandcastle/base/Dockerfile.claude \
  sandcastle/base

# Build Antigravity CLI base image directly from GitHub Release
docker build \
  --tag sandcastle-base:agy \
  --build-arg AGENT_UID="$(id -u)" \
  --build-arg AGENT_GID="$(id -g)" \
  --build-arg TM_VERSION="latest" \
  --file sandcastle/base/Dockerfile.agy \
  sandcastle/base
```
```

# Build Antigravity CLI base image
docker build \
  --tag sandcastle-base:agy \
  --build-arg AGENT_UID="$(id -u)" \
  --build-arg AGENT_GID="$(id -g)" \
  --file sandcastle/base/Dockerfile.agy \
  .
```

---

## Inheriting from the Base Images

To build a project-specific development image, start your project `Dockerfile` with:

```dockerfile
# Choose your preferred agent flavor:
FROM sandcastle-base:claude
# or: FROM sandcastle-base:agy

# Switch to root to install language runtimes and system packages
USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/*

# Switch back to the non-root agent user
USER agent
WORKDIR /home/agent/workspace

# Keep container alive for Sandcastle runner
ENTRYPOINT ["sleep", "infinity"]
```

See the concrete example in [`sandcastle/examples/custom-project/`](../examples/custom-project/) for a complete, runnable setup.
