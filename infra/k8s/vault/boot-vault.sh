#!/usr/bin/env bash
set -e

helm uninstall vault && kubectl delete pvc data-vault-0
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$DIR/../../.."
KEYS_FILE="$PROJECT_ROOT/certs/cluster-keys.json"

echo "Deploying Vault Infrastructure (Helm)"
kubectl create configmap vault-config \
  --from-file=vault.hcl="$DIR/config/vault.hcl" \
  -n default \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl annotate configmap vault-config meta.helm.sh/release-name=vault --overwrite
kubectl annotate configmap vault-config meta.helm.sh/release-namespace=default --overwrite
kubectl label configmap vault-config app.kubernetes.io/managed-by=Helm --overwrite

helm upgrade --install vault hashicorp/vault -f "$DIR/vault-values.yaml" --wait

echo "2. Handling Vault Security State"
echo "Waiting for pod to be ready"
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/vault-0 --timeout=60s

kubectl exec vault-0 -- vault operator init -key-shares=1 -key-threshold=1 -format=json > $KEYS_FILE
UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0]' "$KEYS_FILE" | tr -d '[:space:]')

echo "Unsealing Vault..."
kubectl exec vault-0 -- vault operator unseal "$UNSEAL_KEY"



kubectl port-forward --address 127.0.0.1 pod/vault-0 8200:8200 >/dev/null 2>&1 &
PF_PID=$!
trap "kill $PF_PID" EXIT

echo "3. Applying Terraform (PKI & Secrets)"
cd "$PROJECT_ROOT/infra/terraform"
terraform init
terraform apply -auto-approve

echo "5. Syncing Vault Token to Application Namespace"
APP_NS="sentinel-control-plane"
kubectl create namespace $APP_NS --dry-run=client -o yaml | kubectl apply -f -

ROOT_TOKEN=$(jq -r '.root_token' "$KEYS_FILE")

kubectl create secret generic control-plane-vault-conf \
  --from-literal=vault-token="$ROOT_TOKEN" \
  -n $APP_NS \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Bootstrap Complete"
