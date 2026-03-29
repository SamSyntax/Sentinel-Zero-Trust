#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${DIR}/../.." && pwd)"

echo "Building users-service image..."
cd "${PROJECT_ROOT}/dummy-services/users-service"
docker build -t users-service:latest .
cd "${PROJECT_ROOT}"
kind load docker-image users-service:latest --name "${CLUSTER_NAME}"

echo "Deploying Users Service..."
kubectl apply -f "${PROJECT_ROOT}/dummy-services/users-service/k8s/"

helm upgrade --install data-plane "${DIR}/data-plane" \
  -n sentinel-data-plane --create-namespace \
  --set service.type=NodePort --set service.nodePort=30443
