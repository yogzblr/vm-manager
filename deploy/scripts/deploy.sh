#!/bin/bash
set -euo pipefail

# Deployment script for VM Manager
# Usage: ./deploy.sh [environment] [action]
#   environment: development, production (default: development)
#   action: apply, delete, diff (default: apply)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
K8S_DIR="${ROOT_DIR}/deploy/kubernetes"

ENVIRONMENT="${1:-development}"
ACTION="${2:-apply}"

echo "=== VM Manager Deployment Script ==="
echo "Environment: ${ENVIRONMENT}"
echo "Action: ${ACTION}"
echo ""

# Validate environment
if [ ! -d "${K8S_DIR}/overlays/${ENVIRONMENT}" ]; then
    echo "Error: Unknown environment '${ENVIRONMENT}'"
    echo "Available environments:"
    ls -1 "${K8S_DIR}/overlays/"
    exit 1
fi

# Check for required tools
if ! command -v kubectl &> /dev/null; then
    echo "Error: kubectl is required but not installed"
    exit 1
fi

if ! command -v kustomize &> /dev/null; then
    echo "Warning: kustomize not found, using kubectl kustomize"
    KUSTOMIZE="kubectl kustomize"
else
    KUSTOMIZE="kustomize build"
fi

OVERLAY_DIR="${K8S_DIR}/overlays/${ENVIRONMENT}"

case "${ACTION}" in
    apply)
        echo "Applying configuration..."
        ${KUSTOMIZE} "${OVERLAY_DIR}" | kubectl apply -f -
        echo ""
        echo "Waiting for deployments to be ready..."
        kubectl -n vm-manager wait --for=condition=available --timeout=300s deployment/control-plane || true
        kubectl -n vm-manager wait --for=condition=available --timeout=300s deployment/piko || true
        echo ""
        echo "Deployment status:"
        kubectl -n vm-manager get pods
        ;;
    delete)
        echo "Deleting configuration..."
        ${KUSTOMIZE} "${OVERLAY_DIR}" | kubectl delete -f - --ignore-not-found
        ;;
    diff)
        echo "Showing diff..."
        ${KUSTOMIZE} "${OVERLAY_DIR}" | kubectl diff -f - || true
        ;;
    render)
        echo "Rendering manifests..."
        ${KUSTOMIZE} "${OVERLAY_DIR}"
        ;;
    status)
        echo "Deployment status:"
        kubectl -n vm-manager get all
        echo ""
        echo "Pod details:"
        kubectl -n vm-manager describe pods
        ;;
    *)
        echo "Unknown action: ${ACTION}"
        echo "Usage: $0 [environment] [apply|delete|diff|render|status]"
        exit 1
        ;;
esac

echo ""
echo "=== Operation Complete ==="
