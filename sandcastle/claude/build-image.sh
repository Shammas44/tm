#!/usr/bin/env bash
# ==============================================================================
# Build Script: Sandcastle Claude Code Sandbox Image
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TM_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-sandcastle:claude-dev}"
DOCKERFILE="${SCRIPT_DIR}/Dockerfile"
TM_LINUX_BIN="${TM_ROOT}/bin/tm-linux-amd64"

echo "=================================================================="
echo " Building Sandcastle Image: ${IMAGE_NAME}"
echo "=================================================================="

# 1. Verify / Build Static TaskManager Linux Binary
if [ ! -f "${TM_LINUX_BIN}" ]; then
    echo "==> Static TaskManager Linux binary not found at ${TM_LINUX_BIN}."
    echo "==> Compiling via make build-linux..."
    if [ -f "${TM_ROOT}/Makefile" ]; then
        (cd "${TM_ROOT}" && make build-linux)
    else
        echo "ERROR: Could not locate Makefile at ${TM_ROOT}." >&2
        exit 1
    fi
else
    echo "==> Static Linux binary found at ${TM_LINUX_BIN}."
fi

# 2. Resolve Host UID and GID for Seamless Bind-Mount Permissions
AGENT_UID="$(id -u)"
AGENT_GID="$(id -g)"

echo "==> Host User Mapping: UID=${AGENT_UID}, GID=${AGENT_GID}"
echo "==> Dockerfile: ${DOCKERFILE}"

# 3. Build Docker Image
docker build \
    --tag "${IMAGE_NAME}" \
    --build-arg AGENT_UID="${AGENT_UID}" \
    --build-arg AGENT_GID="${AGENT_GID}" \
    --file "${DOCKERFILE}" \
    "${SCRIPT_DIR}"

echo ""
echo "=================================================================="
echo " Sandbox image ready: ${IMAGE_NAME}"
echo "=================================================================="
echo "Run interactive agent session:"
echo "  cd ${SCRIPT_DIR}"
echo "  tsx interactive.ts"
echo ""
echo "Open a shell in a running container:"
echo "  ./shell.sh"
echo "=================================================================="
