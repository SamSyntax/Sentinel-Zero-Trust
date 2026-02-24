#!/usr/bin/env bash

set -e

echo "Waiting for Vault Pod to be ready"
kubectl wait --for=condition=ready pod/vault-0 --timeout=90s

echo "Configuring PKI Engine inside Vault"
kubectl exec vault-0 -- sh -c '
  export VAULT_TOKEN=root
  export VAULT_ADDR=http://127.0.0.1:8200

  vault secrets enable pki
  vault secrets tune -max-lease-ttl=87600h pki

  vault write -field=certificate pki/root/generate/internal \
    common_name="Sentinel Root CA" \
    ttl=87600h > /tmp/root_ca.crt

  vault write pki/roles/sentinel-service \
    allowed_domains="sentinel.local,sentinel-zt" \
    allow_subdomains=true \
    max_ttl="72h" \
    generate_lease=true
'

echo "Extracting Root CA and creating Kubernetes Secret"
mkdir -p certs
kubectl cp vault-0:/tmp/root_ca.crt ./certs/root_ca.crt

kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt=./certs/root_ca.crt \
  --dry-run=client -o yaml | kubectl apply -f -

echo "PKI Engine configured successfully"
