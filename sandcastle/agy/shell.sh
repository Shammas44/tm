#!/usr/bin/env bash
# ==============================================================================
# Shell Script: Connect to running Sandcastle container for manual execution
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TM_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-sandcastle:agy-dev}"

echo "=================================================================="
echo " Looking for running Sandcastle container (${IMAGE_NAME})..."
echo "=================================================================="

# Find container ID running the image
CONTAINER_ID="$(docker ps --filter "ancestor=${IMAGE_NAME}" --format "{{.ID}}" | head -n 1 || true)"

if [ -n "${CONTAINER_ID}" ]; then
    CONTAINER_NAME="$(docker inspect --format '{{.Name}}' "${CONTAINER_ID}" | sed 's/^\///')"
    echo "==> Found running container: ${CONTAINER_NAME} (${CONTAINER_ID})"
    echo "==> Attaching interactive bash shell as 'agent' user in /home/agent/workspace..."
    echo "=================================================================="
    exec docker exec -it -u agent -w /home/agent/workspace "${CONTAINER_ID}" /bin/bash
else
    echo "No running container found for image: ${IMAGE_NAME}."
    echo ""
    echo "Options:"
    echo "  1) Launch an interactive agent session first:"
    echo "       cd ${SCRIPT_DIR}"
    echo "       tsx interactive.ts"
    echo ""
    echo "  2) Or launch a standalone interactive debugging shell right now:"
    read -r -p "     Launch temporary debug container? [y/N]: " CONFIRM
    if [[ "${CONFIRM}" =~ ^[Yy]$ ]]; then
        TM_LINUX_BIN="${TM_ROOT}/bin/tm-linux-amd64"
        HOST_PROJECT_DIR="${HOME}/.taskmanager/projects/debug"
        mkdir -p "${HOST_PROJECT_DIR}"

        MOUNTS=()
        if [ -f "${TM_LINUX_BIN}" ]; then
            MOUNTS+=(-v "${TM_LINUX_BIN}:/home/agent/.local/bin/tm:ro")
        fi
        MOUNTS+=(-v "${HOST_PROJECT_DIR}:/home/agent/.taskmanager/projects/debug")

        exec docker run --rm -it \
            -u agent \
            -w /home/agent/workspace \
            "${MOUNTS[@]}" \
            "${IMAGE_NAME}" /bin/bash
    else
        exit 0
    fi
fi
