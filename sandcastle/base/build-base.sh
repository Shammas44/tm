#!/usr/bin/env bash
# ==============================================================================
# Build Script: Sandcastle Base Images (Claude Code & Antigravity CLI)
# ==============================================================================
# Usage:
#   ./build-base.sh              # Build both images using latest GitHub release
#   ./build-base.sh claude       # Build only sandcastle-base:claude
#   ./build-base.sh agy          # Build only sandcastle-base:agy
#   TM_VERSION=v0.1.0 ./build-base.sh   # Build against specific release version
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

TARGET="${1:-all}"
TM_VERSION="${TM_VERSION:-latest}"
AGENT_UID="$(id -u)"
AGENT_GID="$(id -g)"

echo "=================================================================="
echo " Building Sandcastle Base Images (GitHub Release Only)"
echo " tm Release Version: ${TM_VERSION}"
echo " Host User Mapping:  UID=${AGENT_UID}, GID=${AGENT_GID}"
echo "=================================================================="

build_claude() {
    echo ""
    echo "==> Building sandcastle-base:claude from GitHub release..."
    docker build \
        --tag "sandcastle-base:claude" \
        --build-arg AGENT_UID="${AGENT_UID}" \
        --build-arg AGENT_GID="${AGENT_GID}" \
        --build-arg TM_VERSION="${TM_VERSION}" \
        --file "${SCRIPT_DIR}/Dockerfile.claude" \
        "${SCRIPT_DIR}"
    echo "==> Successfully built sandcastle-base:claude"
}

build_agy() {
    echo ""
    echo "==> Building sandcastle-base:agy from GitHub release..."
    docker build \
        --tag "sandcastle-base:agy" \
        --build-arg AGENT_UID="${AGENT_UID}" \
        --build-arg AGENT_GID="${AGENT_GID}" \
        --build-arg TM_VERSION="${TM_VERSION}" \
        --file "${SCRIPT_DIR}/Dockerfile.agy" \
        "${SCRIPT_DIR}"
    echo "==> Successfully built sandcastle-base:agy"
}

case "${TARGET}" in
    claude)
        build_claude
        ;;
    agy)
        build_agy
        ;;
    all)
        build_claude
        build_agy
        ;;
    *)
        echo "Error: Unknown target '${TARGET}'. Use 'claude', 'agy', or 'all'." >&2
        exit 1
        ;;
esac

echo ""
echo "=================================================================="
echo " Sandcastle base image build complete!"
echo " Images available:"
echo "   - sandcastle-base:claude"
echo "   - sandcastle-base:agy"
echo "=================================================================="
