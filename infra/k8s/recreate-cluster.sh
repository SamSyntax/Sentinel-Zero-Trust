#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
REGISTRY="localhost:5000"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${DIR}/../.." && pwd)"
ARGO_FW_PORT=8081
REPO_KEY_PATH="/home/sam/.ssh/argocd/argo_key"
REPO_URL="git@github.com:SamSyntax/Sentinel-Zero-Trust.git"

echo "Deleting Kind Cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}" || true

echo "Creating Kind Cluster: ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --config "${DIR}/kind/kind-config.yaml"

echo "Deploying in-cluster registry"
kubectl apply -f "${DIR}/registry/registry-deployment.yaml"
kubectl wait --for=condition=available -n kube-system deployment/registry --timeout=60s

kubectl create namespace sentinel-data-plane --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace sentinel-control-plane --dry-run=client -o yaml | kubectl apply -f -

bash "${DIR}/vault/boot-vault.sh" -a -p 8210

echo "Verifying Vault bootstrap health..."
VAULT_STATUS_JSON=$(kubectl exec vault-0 -- vault status -format=json 2>/dev/null || true)
if ! jq -e . >/dev/null 2>&1 <<<"$VAULT_STATUS_JSON"; then
  echo "Vault status output is invalid after bootstrap"
  exit 1
fi

if [[ "$(jq -r '.initialized' <<<"$VAULT_STATUS_JSON")" != "true" || "$(jq -r '.sealed' <<<"$VAULT_STATUS_JSON")" != "false" ]]; then
  echo "Vault is not ready after bootstrap. Expected initialized=true and sealed=false."
  exit 1
fi

bash "${DIR}/observability/setup-observability.sh"

echo "Bootstrapping Vault PKI and Webhook TLS..."
bash "${DIR}/cluster-pki-init.sh"
bash "${DIR}/init-webhook-tls.sh"

echo "Building and pushing sentinel-control-plane image"
docker build -t "${REGISTRY}/sentinel-control-plane:latest" "${PROJECT_ROOT}/control-plane/"
docker push "${REGISTRY}/sentinel-control-plane:latest"
helm upgrade --install control-plane "${DIR}/control-plane" \
  -n sentinel-control-plane --create-namespace \
  --wait

# echo "Registering Sidecar Injection webhook..."
# export CA_BUNDLE=$(cat "${PROJECT_ROOT}/certs/root_ca_base64.txt")
# envsubst < "${DIR}/control-plane/webhook-registration.yaml" | kubectl apply -f -

kubectl label namespace default sentinel-zt.io/injection=enabled --overwrite

echo "Building and pushing sentinel-init image..."
cd "${DIR}/../../sentinel-init"
docker build -t kind.local/sentinel-init:latest .
docker tag kind.local/sentinel-init:latest "${REGISTRY}/sentinel-init:latest"
docker tag kind.local/sentinel-init:latest "${REGISTRY}/sentinel-init:redirectfix"
docker push "${REGISTRY}/sentinel-init:latest"
docker push "${REGISTRY}/sentinel-init:redirectfix"
cd "${PROJECT_ROOT}"

echo "Building and pushing users-service image..."
cd "${PROJECT_ROOT}/dummy-services/users-service"
docker build -t users-service:latest .
docker tag users-service:latest "${REGISTRY}/users-service:latest"
docker push "${REGISTRY}/users-service:latest"
cd "${PROJECT_ROOT}"

echo "Building and pushing echo-service image..."
cd "${PROJECT_ROOT}/dummy-services/echo-service"
docker build -t echo-service:latest .
docker tag echo-service:latest "${REGISTRY}/echo-service:latest"
docker push "${REGISTRY}/echo-service:latest"
cd "${PROJECT_ROOT}"

echo "Deploying PostgreSQL..."
bash "${PROJECT_ROOT}/dummy-services/psql-k8s/psql-helm.sh" database

echo "Deploying Users Service..."
kubectl create namespace users-service --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace users-service sentinel-zt.io/injection=enabled --overwrite
kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt="${PROJECT_ROOT}/certs/root_ca.crt" \
  --dry-run=client --namespace=users-service -o yaml | kubectl apply -f -
kubectl -n users-service apply -f "${PROJECT_ROOT}/dummy-services/users-service/k8s/"

echo "Deploying echo-service..."
kubectl create namespace echo-service --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace echo-service sentinel-zt.io/injection=enabled --overwrite
kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt="${PROJECT_ROOT}/certs/root_ca.crt" \
  --dry-run=client --namespace=echo-service -o yaml | kubectl apply -f -
kubectl -n echo-service apply -f "${PROJECT_ROOT}/dummy-services/echo-service/k8s/"

echo "Building and pushing sentinel-data-plane image"
docker build -t "${REGISTRY}/sentinel-data-plane:latest" "${PROJECT_ROOT}/data-plane/"
docker tag "${REGISTRY}/sentinel-data-plane:latest" "${REGISTRY}/sentinel-data-plane:redirectfix"
docker push "${REGISTRY}/sentinel-data-plane:latest"
docker push "${REGISTRY}/sentinel-data-plane:redirectfix"
# helm upgrade --install data-plane "${DIR}/data-plane" \
#   -n sentinel-data-plane --create-namespace \
#   --set service.type=NodePort --set service.nodePort=30443

# echo "Deploying ArgoCD..."
# kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
# kubectl apply -n argocd --server-side --force-conflicts -f https://raw.githubusercontent.com/argoproj/argo-cd/master/manifests/install.yaml
# echo "Waiting for ArgoCD to be ready..."
# # Wait for the ArgoCD server deployment to be available
# kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=300s
#
# # Now wait for the initial admin secret to be created
# echo "Waiting for ArgoCD initial admin secret..."
# until kubectl get secret -n argocd argocd-initial-admin-secret >/dev/null 2>&1; do
#   echo "Waiting for argocd-initial-admin-secret to be created..."
#   sleep 5
# done
#
# kubectl port-forward svc/argocd-server -n argocd $ARGO_FW_PORT:443 >/dev/null 2>&1 &
# PF_PID=$!
# trap "kill $PF_PID 2>/dev/null || true" EXIT
#
# # Now get the password
# argo_pass=$(kubectl get secret -n argocd argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)
# argocd login localhost:8081 --insecure --username admin --password "$argo_pass"
#
# argocd repo add "$REPO_URL" \
#   --ssh-private-key-path "$REPO_KEY_PATH" \
#   --insecure-skip-server-verification
#
# kubectl apply -f "${PROJECT_ROOT}/argocd/app-of-apps.yaml"
# echo "ArgoCD Initial Password: $argo_pass"
