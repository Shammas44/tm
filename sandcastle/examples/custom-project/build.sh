#!/usr/bin/env bash
# ==============================================================================
# Build Script: Example Project Image Extending Sandcastle Base
# ==============================================================================
# Usage:
#   ./build.sh           # Builds my-project:claude-dev using sandcastle-base:claude
#   ./build.sh claude    # Builds my-project:claude-dev
#   ./build.sh agy       # Builds my-project:agy-dev
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLAVOR="${1:-claude}"
IMAGE_TAG="my-project:${FLAVOR}-dev"
BASE_IMAGE="sandcastle-base:${FLAVOR}"

echo "=================================================================="
echo " Building Project Sandbox Image"
echo " Flavor:     ${FLAVOR}"
echo " Base Image: ${BASE_IMAGE}"
echo " Image Tag:  ${IMAGE_TAG}"
echo "=================================================================="

# 1. Verify the required base image exists
if ! docker image inspect "${BASE_IMAGE}" >/dev/null 2>&1; then
    echo "==> Base image '${BASE_IMAGE}' not found locally."
    echo "==> Building base image first..."
    bash "${SCRIPT_DIR}/../../base/build-base.sh" "${FLAVOR}"
fi

# 2. Build the project-specific image
docker build \
    --build-arg AGENT_FLAVOR="${FLAVOR}" \
    --tag "${IMAGE_TAG}" \
    --file "${SCRIPT_DIR}/Dockerfile" \
    "${SCRIPT_DIR}"

echo ""
echo "=================================================================="
echo " Project image successfully built: ${IMAGE_TAG}"
echo " To run a shell or launch an agent, run: ./run.sh ${FLAVOR}"
echo "=================================================================="
