#!/usr/bin/env bash
set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

kubectl create configmap vault-config \
  --from-file=vault.hcl="$DIR/config/vault.hcl" \
  -n default \
  --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install vault hashicorp/vault -f "$DIR/vault-values.yaml" --wait

INIT_STATUS=$(kubectl exec vault-0 -- vault status -format=json | jq -r '.initialized')

if [ "$INIT_STATUS" == "false" ]; then
    echo "Initializing Vault..."
    kubectl exec vault-0 -- vault operator init -key-shares=1 -key-threshold=1 -format=json > "$DIR/cluster-keys.json"
fi

UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0]' "$DIR/cluster-keys.json")
kubectl exec vault-0 -- vault operator unseal "$UNSEAL_KEY"

echo "Vault is unsealed and ready."
