# ==============================================================================
# Example Project Dockerfile: Extending Sandcastle Base Image
# ==============================================================================
# You can choose either 'claude' or 'agy' as the agent flavor:
#   docker build --build-arg AGENT_FLAVOR=claude -t my-project:claude-dev .
#   docker build --build-arg AGENT_FLAVOR=agy    -t my-project:agy-dev .
# ==============================================================================
ARG AGENT_FLAVOR=claude
FROM sandcastle-base:${AGENT_FLAVOR}

# Switch to root to install project-specific system libraries and compilers
USER root

# Example: Install Python 3, pip, venv, and development tools for a Python API project
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-venv \
    python3-dev \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# (Optional) Pre-install common project CLI tools or test runners
RUN python3 -m pip install --no-cache-dir --break-system-packages \
    pytest \
    ruff \
    black

# ==============================================================================
# [PROJECT-SPECIFIC SETUP]
# Copy or configure any project-level configurations, certificates, or assets:
# RUN mkdir -p /opt/project-deps && chown -R agent:agent /opt/project-deps
# ==============================================================================

# Switch back to the non-root 'agent' user
USER agent
WORKDIR /home/agent/workspace

# Keep container alive so Sandcastle or docker exec can execute against it
ENTRYPOINT ["sleep", "infinity"]
