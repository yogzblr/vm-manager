#!/bin/bash
set -euo pipefail

# Push script for VM Manager Docker images
# Usage: ./push.sh [component] [version]

COMPONENT="${1:-all}"
VERSION="${2:-latest}"

REGISTRY="${DOCKER_REGISTRY:-ghcr.io/yourorg}"

echo "=== VM Manager Push Script ==="
echo "Component: ${COMPONENT}"
echo "Version: ${VERSION}"
echo "Registry: ${REGISTRY}"
echo ""

push_image() {
    local image=$1
    echo "Pushing ${REGISTRY}/${image}:${VERSION}..."
    docker push "${REGISTRY}/${image}:${VERSION}"

    if [ "${VERSION}" != "latest" ]; then
        echo "Pushing ${REGISTRY}/${image}:latest..."
        docker push "${REGISTRY}/${image}:latest"
    fi
}

case "${COMPONENT}" in
    vm-agent)
        push_image "vm-agent"
        ;;
    control-plane)
        push_image "control-plane"
        ;;
    all)
        push_image "vm-agent"
        push_image "control-plane"
        ;;
    *)
        echo "Unknown component: ${COMPONENT}"
        echo "Usage: $0 [vm-agent|control-plane|all] [version]"
        exit 1
        ;;
esac

echo ""
echo "=== Push Complete ==="
