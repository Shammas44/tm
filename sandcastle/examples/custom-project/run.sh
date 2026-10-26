#!/usr/bin/env bash
# ==============================================================================
# Run Script: Interactive Shell & Agent Runner for Custom Project
# ==============================================================================
# Usage:
#   ./run.sh             # Launches interactive shell with Claude flavor
#   ./run.sh claude      # Launches Claude container
#   ./run.sh agy         # Launches AGY container
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLAVOR="${1:-claude}"
IMAGE_NAME="my-project:${FLAVOR}-dev"
PROJECT_NAME="my-project"

HOST_PROJECT_DB_DIR="${HOME}/.taskmanager/projects/${PROJECT_NAME}"
mkdir -p "${HOST_PROJECT_DB_DIR}"

# Prepare bind mounts
MOUNTS=(
    # 1. Mount current workspace
    -v "${SCRIPT_DIR}:/home/agent/workspace"
    # 2. Mount persistent TaskManager project state
    -v "${HOST_PROJECT_DB_DIR}:/home/agent/.taskmanager/projects/${PROJECT_NAME}"
)

# 3. Mount agent authentication files from host if present
if [ "${FLAVOR}" = "claude" ]; then
    if [ -d "${HOME}/.claude" ]; then
        MOUNTS+=(-v "${HOME}/.claude:/home/agent/.claude")
    fi
elif [ "${FLAVOR}" = "agy" ]; then
    if [ -d "${HOME}/.gemini" ]; then
        MOUNTS+=(-v "${HOME}/.gemini:/home/agent/.gemini")
    fi
fi

echo "=================================================================="
echo " Launching Project Container: ${IMAGE_NAME}"
echo " Persistent Tasks DB: ${HOST_PROJECT_DB_DIR}"
echo " Working Directory:   /home/agent/workspace"
echo "=================================================================="

exec docker run --rm -it \
    -u agent \
    -e TASKMANAGER_PROJECT="${PROJECT_NAME}" \
    -w /home/agent/workspace \
    "${MOUNTS[@]}" \
    "${IMAGE_NAME}" \
    /bin/bash
