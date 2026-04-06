#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
REGISTRY="${REGISTRY:-localhost:5000}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${DIR}/../.." && pwd)"

echo "Deleting Kind Cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}" || true

echo "Starting local registry..."
bash "${DIR}/registry/create-registry.sh"

echo "Creating Kind Cluster: ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --config "${DIR}/kind/kind-config.yaml"

echo "Connecting registry to Kind network..."
docker network connect "kind" "kind-registry" || true

kubectl create namespace sentinel-data-plane --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace sentinel-control-plane --dry-run=client -o yaml | kubectl apply -f -

bash "${DIR}/vault/boot-vault.sh" -a -p 8210
bash "${DIR}/observability/setup-observability.sh"

echo "Bootstrapping Vault PKI and Webhook TLS..."
bash "${DIR}/cluster-pki-init.sh"
bash "${DIR}/init-webhook-tls.sh"

helm upgrade --install control-plane "${DIR}/control-plane" \
  -n sentinel-control-plane --create-namespace \
  --wait

echo "Registering Sidecar Injection webhook..."
export CA_BUNDLE=$(cat "${PROJECT_ROOT}/certs/issuing_ca_base64.txt")
envsubst < "${DIR}/control-plane/webhook-registration.yaml" | kubectl apply -f -

kubectl label namespace default sentinel-zt.io/injection=enabled --overwrite

echo "Building and pushing sentinel-init image..."
cd "${DIR}/sentinel-init"
docker build -t kind.local/sentinel-init:latest .
docker tag kind.local/sentinel-init:latest "${REGISTRY}/sentinel-init:latest"
docker push "${REGISTRY}/sentinel-init:latest"
cd "${PROJECT_ROOT}"

echo "Building and pushing users-service image..."
cd "${PROJECT_ROOT}/dummy-services/users-service"
docker build -t users-service:latest .
docker tag users-service:latest "${REGISTRY}/users-service:latest"
docker push "${REGISTRY}/users-service:latest"
cd "${PROJECT_ROOT}"

echo "Deploying PostgreSQL..."
bash "${PROJECT_ROOT}/dummy-services/psql-k8s/psql-helm.sh" default

echo "Deploying Users Service..."
kubectl apply -f "${PROJECT_ROOT}/dummy-services/users-service/k8s/"

helm upgrade --install data-plane "${DIR}/data-plane" \
  -n sentinel-data-plane --create-namespace \
  --set service.type=NodePort --set service.nodePort=30443
