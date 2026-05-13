#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${DIR}/../../../" && pwd)"
REGISTRY="localhost:5000"

users "Building users-service image..."
cd "${PROJECT_ROOT}/dummy-services/users-service"
docker build -t ${REGISTRY}/users-service:latest .
docker push ${REGISTRY}/users-service:latest
cd "${PROJECT_ROOT}"
kind load docker-image ${REGISTRY}/users-service:latest --name "${CLUSTER_NAME}" || true

users "Deploying users-service..."
kubectl create namespace users-service --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace users-service sentinel-zt.io/injection=enabled --overwrite
kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt="${PROJECT_ROOT}/certs/root_ca.crt" \
  --dry-run=client --namespace=users-service -o yaml | kubectl apply -f -
kubectl apply -f "${PROJECT_ROOT}/dummy-services/users-service/k8s/"
