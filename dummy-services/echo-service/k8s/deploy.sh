#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${DIR}/../../../" && pwd)"
REGISTRY="localhost:5000"

echo "Building echo-service image..."
cd "${PROJECT_ROOT}/dummy-services/echo-service"
docker build -t ${REGISTRY}/echo-service:latest .
docker push ${REGISTRY}/echo-service:latest
cd "${PROJECT_ROOT}"
kind load docker-image ${REGISTRY}/echo-service:latest --name "${CLUSTER_NAME}" || true

echo "Deploying echo-service..."
kubectl create namespace echo-service --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace echo-service sentinel-zt.io/injection=enabled --overwrite
kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt="${PROJECT_ROOT}/certs/root_ca.crt" \
  --dry-run=client --namespace=echo-service -o yaml | kubectl apply -f -
kubectl apply -f "${PROJECT_ROOT}/dummy-services/echo-service/k8s/"
