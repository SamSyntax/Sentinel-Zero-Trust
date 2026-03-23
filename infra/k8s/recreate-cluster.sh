#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${DIR}/../.." && pwd)"

echo "Deleting Kind Cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}" || true

bash "${DIR}/kind/setup-cluster.sh"
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

echo "Building sentinel-init image..."
cd "${DIR}/sentinel-init"
docker build -t kind.local/sentinel-init:latest .
cd "${PROJECT_ROOT}"
kind load docker-image kind.local/sentinel-init:latest --name "${CLUSTER_NAME}"

echo "Building users-service image..."
cd "${PROJECT_ROOT}/dummy-services/users-service"
docker build -t users-service:latest .
cd "${PROJECT_ROOT}"
kind load docker-image users-service:latest --name "${CLUSTER_NAME}"

echo "Deploying PostgreSQL..."
bash "${PROJECT_ROOT}/dummy-services/psql-k8s/psql-helm.sh" default

echo "Deploying Users Service..."
kubectl apply -f "${PROJECT_ROOT}/dummy-services/users-service/k8s/"

helm upgrade --install data-plane "${DIR}/data-plane" \
  -n sentinel-data-plane --create-namespace \
  --set service.type=NodePort --set service.nodePort=30443
