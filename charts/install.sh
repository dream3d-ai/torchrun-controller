#!/bin/bash
# Install script for torchrun-controller Helm chart

set -e

# Default values
NAMESPACE="torchrun-system"
RELEASE_NAME="torchrun-controller"
VALUES_FILE=""
DRY_RUN=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -n|--namespace)
      NAMESPACE="$2"
      shift 2
      ;;
    -r|--release)
      RELEASE_NAME="$2"
      shift 2
      ;;
    -f|--values)
      VALUES_FILE="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN="--dry-run"
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [OPTIONS]"
      echo "Options:"
      echo "  -n, --namespace <namespace>  Namespace to install into (default: torchrun-system)"
      echo "  -r, --release <name>         Release name (default: torchrun-controller)"
      echo "  -f, --values <file>          Values file to use"
      echo "  --dry-run                    Simulate installation"
      echo "  -h, --help                   Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Get the directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
CHART_DIR="$SCRIPT_DIR/torchrun-controller"

echo "Installing torchrun-controller..."
echo "  Namespace: $NAMESPACE"
echo "  Release: $RELEASE_NAME"
echo "  Chart: $CHART_DIR"

# Check if helm is installed
if ! command -v helm &> /dev/null; then
    echo "Error: helm is not installed. Please install helm first."
    exit 1
fi

# Create namespace if it doesn't exist (unless dry-run)
if [[ -z "$DRY_RUN" ]]; then
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
fi

# Build helm install command
HELM_CMD="helm upgrade --install $RELEASE_NAME $CHART_DIR"
HELM_CMD="$HELM_CMD --namespace $NAMESPACE"
HELM_CMD="$HELM_CMD --create-namespace"

if [[ -n "$VALUES_FILE" ]]; then
    HELM_CMD="$HELM_CMD --values $VALUES_FILE"
fi

if [[ -n "$DRY_RUN" ]]; then
    HELM_CMD="$HELM_CMD $DRY_RUN"
fi

# Execute helm install
echo "Running: $HELM_CMD"
eval $HELM_CMD

if [[ -z "$DRY_RUN" ]]; then
    echo ""
    echo "Installation complete!"
    echo ""
    echo "To check the status:"
    echo "  kubectl get pods -n $NAMESPACE"
    echo ""
    echo "To view queues:"
    echo "  kubectl get torchrunqueues --all-namespaces"
    echo ""
    echo "To uninstall:"
    echo "  helm uninstall $RELEASE_NAME -n $NAMESPACE"
fi 