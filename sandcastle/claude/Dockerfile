# ==============================================================================
# Language-Agnostic Sandcastle Sandbox with Claude Code CLI & TaskManager
# ==============================================================================
# Base: Node 22 Bookworm Slim
FROM node:22-bookworm-slim

ARG AGENT_UID=1000
ARG AGENT_GID=1000

ENV DEBIAN_FRONTEND=noninteractive \
    PATH="/usr/local/bin:/home/agent/.local/bin:$PATH" \
    EDITOR=vim \
    VISUAL=vim

# 1. Install core system utilities, git, sudo, curl, jq, GitHub CLI, and Vim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    gnupg \
    jq \
    sudo \
    build-essential \
    vim \
    && mkdir -p -m 755 /etc/apt/keyrings \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/* \
    && update-alternatives --set editor /usr/bin/vim.basic 2>/dev/null || true

# 2. Install Anthropic Claude Code CLI globally
RUN npm install -g @anthropic-ai/claude-code

# ==============================================================================
# [PROJECT EXTENSION POINT]
# Add your project-specific toolchain, compilers, or language runtimes here:
# Examples:
#   - Rust:   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
#   - Go:     apt-get install -y golang
#   - Python: apt-get install -y python3 python3-pip python3-venv
#   - C/C++:  apt-get install -y clang cmake ninja-build
# ==============================================================================

# 3. Create non-root `agent` user matching host UID/GID with passwordless sudo
RUN (groupadd -g "${AGENT_GID}" agent 2>/dev/null || groupmod -n agent "$(getent group "${AGENT_GID}" | cut -d: -f1)") && \
    (useradd -m -u "${AGENT_UID}" -g "${AGENT_GID}" -s /bin/bash agent 2>/dev/null || \
     usermod -l agent -d /home/agent -m "$(getent passwd "${AGENT_UID}" | cut -d: -f1)") && \
    echo "agent ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/agent && \
    chmod 0440 /etc/sudoers.d/agent && \
    mkdir -p /home/agent/.claude/projects \
             /home/agent/.taskmanager/projects \
             /home/agent/.local/bin \
             /home/agent/workspace && \
    echo "export EDITOR=vim" >> /home/agent/.bashrc && \
    echo "export VISUAL=vim" >> /home/agent/.bashrc && \
    chown -R "${AGENT_UID}:${AGENT_GID}" /home/agent

USER agent
WORKDIR /home/agent/workspace

# Keep container alive so Sandcastle runner can docker exec into it
ENTRYPOINT ["sleep", "infinity"]
