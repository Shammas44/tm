# ==============================================================================
# Language-Agnostic Sandcastle Sandbox with Antigravity CLI & TaskManager
# ==============================================================================
# Base: Debian Bookworm Slim
FROM debian:bookworm-slim

ARG AGENT_UID=1000
ARG AGENT_GID=1000
ARG DEFAULT_MODEL="gemini-3.8-flash-medium"

ENV DEBIAN_FRONTEND=noninteractive \
    PATH="/usr/local/bin:/home/agent/.local/bin:$PATH" \
    EDITOR=vim \
    VISUAL=vim \
    AGY_MODEL="${DEFAULT_MODEL}"

# 1. Install core system utilities, git, sudo, curl, jq, and Vim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    gnupg \
    jq \
    sudo \
    build-essential \
    vim \
    && rm -rf /var/lib/apt/lists/* \
    && update-alternatives --set editor /usr/bin/vim.basic 2>/dev/null || true

# 2. Install Google Antigravity CLI (`agy`) inside the container
RUN curl -fsSL https://antigravity.google/cli/install.sh | bash -s -- -d /usr/local/bin && \
    chmod +x /usr/local/bin/agy 2>/dev/null || true

# ==============================================================================
# [PROJECT EXTENSION POINT]
# Add your project-specific toolchain, compilers, or language runtimes here:
# Examples:
#   - Rust:   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
#   - Go:     apt-get install -y golang
#   - Python: apt-get install -y python3 python3-pip python3-venv
#   - Node:   curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt-get install -y nodejs
# ==============================================================================

# 3. Create non-root `agent` user matching host UID/GID with passwordless sudo
RUN (groupadd -g "${AGENT_GID}" agent 2>/dev/null || groupmod -n agent "$(getent group "${AGENT_GID}" | cut -d: -f1)") && \
    (useradd -m -u "${AGENT_UID}" -g "${AGENT_GID}" -s /bin/bash agent 2>/dev/null || \
     usermod -l agent -d /home/agent -m "$(getent passwd "${AGENT_UID}" | cut -d: -f1)") && \
    echo "agent ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/agent && \
    chmod 0440 /etc/sudoers.d/agent && \
    mkdir -p /home/agent/.gemini/antigravity-cli/cache \
             /home/agent/.taskmanager/projects \
             /home/agent/.local/bin \
             /home/agent/workspace && \
    echo '{"allowNonWorkspaceAccess":true,"enableTerminalSandbox":false,"model":"'"${DEFAULT_MODEL}"'"}' > /home/agent/.gemini/antigravity-cli/settings.json && \
    echo '{"consumerOnboardingComplete":true,"enterpriseOnboardingComplete":false,"onboardingComplete":true}' > /home/agent/.gemini/antigravity-cli/cache/onboarding.json && \
    echo "export EDITOR=vim" >> /home/agent/.bashrc && \
    echo "export VISUAL=vim" >> /home/agent/.bashrc && \
    chown -R "${AGENT_UID}:${AGENT_GID}" /home/agent

USER agent
WORKDIR /home/agent/workspace

# Keep container alive so Sandcastle runner can docker exec into it
ENTRYPOINT ["sleep", "infinity"]
