#!/bin/bash
set -euo pipefail

# Build script for VM Manager components
# Usage: ./build.sh [component] [version]
#   component: vm-agent, control-plane, or all (default)
#   version: version tag (default: dev)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

COMPONENT="${1:-all}"
VERSION="${2:-dev}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Docker registry
REGISTRY="${DOCKER_REGISTRY:-ghcr.io/yourorg}"

echo "=== VM Manager Build Script ==="
echo "Component: ${COMPONENT}"
echo "Version: ${VERSION}"
echo "Commit: ${COMMIT}"
echo "Build Date: ${BUILD_DATE}"
echo "Registry: ${REGISTRY}"
echo ""

build_vm_agent() {
    echo "Building vm-agent..."
    cd "${ROOT_DIR}/vm-agent"

    docker build \
        --build-arg VERSION="${VERSION}" \
        --build-arg COMMIT="${COMMIT}" \
        --build-arg BUILD_DATE="${BUILD_DATE}" \
        -t "${REGISTRY}/vm-agent:${VERSION}" \
        -t "${REGISTRY}/vm-agent:latest" \
        .

    echo "vm-agent built successfully"
}

build_control_plane() {
    echo "Building control-plane..."
    cd "${ROOT_DIR}/control-plane"

    docker build \
        --build-arg VERSION="${VERSION}" \
        --build-arg COMMIT="${COMMIT}" \
        --build-arg BUILD_DATE="${BUILD_DATE}" \
        -t "${REGISTRY}/control-plane:${VERSION}" \
        -t "${REGISTRY}/control-plane:latest" \
        .

    echo "control-plane built successfully"
}

case "${COMPONENT}" in
    vm-agent)
        build_vm_agent
        ;;
    control-plane)
        build_control_plane
        ;;
    all)
        build_vm_agent
        build_control_plane
        ;;
    *)
        echo "Unknown component: ${COMPONENT}"
        echo "Usage: $0 [vm-agent|control-plane|all] [version]"
        exit 1
        ;;
esac

echo ""
echo "=== Build Complete ==="
echo "Images built:"
docker images | grep -E "(vm-agent|control-plane)" | head -10
