# Extending Sandcastle Base Image for a Custom Project

This example demonstrates how to build a **project-specific Docker image** on top of the Sandcastle base images (`sandcastle-base:claude` or `sandcastle-base:agy`).

Because **TaskManager (`tm`)**, **Vim**, **Git**, **curl**, **jq**, **sudo**, and the agent CLIs are already pre-installed and configured in the base image, your project `Dockerfile` only needs to focus on your project's specific language runtimes and dependencies.

---

## 1. Project Dockerfile Structure

The project `Dockerfile` accepts an `AGENT_FLAVOR` argument (`claude` or `agy`), switches to `root` to install dependencies, and switches back to `agent`:

```dockerfile
# Choose either 'claude' or 'agy'
ARG AGENT_FLAVOR=claude
FROM sandcastle-base:${AGENT_FLAVOR}

# 1. Switch to root to install project-specific packages
USER root

# Example: Python 3 + dev headers + pytest
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-venv \
    python3-dev \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --no-cache-dir --break-system-packages \
    pytest \
    ruff \
    black

# 2. Switch back to the non-root agent user
USER agent
WORKDIR /home/agent/workspace

# Keep container alive for interactive Sandcastle execution
ENTRYPOINT ["sleep", "infinity"]
```

---

## 2. Quickstart

### Step 1: Build the Base Images (one-time setup)

If you haven't already built the base images:

```bash
# From repository root:
bash sandcastle/base/build-base.sh
```

### Step 2: Build Your Project Image

```bash
cd sandcastle/examples/custom-project

# Build with Claude Code flavor (tagged as my-project:claude-dev):
bash build.sh claude

# Or build with Google Antigravity flavor (tagged as my-project:agy-dev):
bash build.sh agy
```

### Step 3: Run an Interactive Development Shell

```bash
# Starts the container with your workspace and task database mounted:
bash run.sh claude
```

### Step 4: Inside the Container

Once inside the container as user `agent`:

```bash
# 1. Verify TaskManager is pre-installed and working
tm --help

# 2. Initialize project working memory
tm init

# 3. Add tasks and start work
tm add "Implement login endpoint" priority:H
tm 1 start
tm 1 log "Testing with pytest"

# 4. Check project status
tm status
tm list

# 5. Launch the AI coding agent directly inside the container:
# If Claude flavor:
claude

# If Antigravity flavor:
agy
```

All task state changes are persisted in `~/.taskmanager/projects/my-project/state.db` on your host machine, surviving container restarts.

---

## 3. Extending with Other Languages

Simply adapt the `RUN apt-get ...` block in `Dockerfile`:

### Go Project
```dockerfile
USER root
RUN apt-get update && apt-get install -y --no-install-recommends golang && rm -rf /var/lib/apt/lists/*
USER agent
```

### Rust Project
```dockerfile
USER agent
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/home/agent/.cargo/bin:${PATH}"
```

### C/C++ Project
```dockerfile
USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
    clang cmake ninja-build gdb valgrind \
    && rm -rf /var/lib/apt/lists/*
USER agent
```
