#!/usr/bin/env bash

set -e

echo "Waiting for Vault Pod to be ready"
kubectl wait --for=condition=ready pod/vault-0 --timeout=90s

echo "Configuring PKI Engine inside Vault"
VAULT_TOKEN=$(jq -r '.root_token' ./certs/cluster-keys.json)
kubectl exec vault-0 -- sh -c "
  export VAULT_TOKEN=${VAULT_TOKEN}
  export VAULT_ADDR=http://127.0.0.1:8200

  vault secrets enable pki
  vault secrets tune -max-lease-ttl=87600h pki

  vault write -field=certificate pki/root/generate/internal \
    common_name=\"Sentinel Root CA\" \
    ttl=87600h > /tmp/root_ca.crt

  vault write pki/roles/sentinel-service \
    allowed_domains=\"sentinel.local,sentinel-zt,svc,cluster.local\" \
    allow_subdomains=true \
    allow_any_name=true \
    max_ttl=\"72h\" \
    generate_lease=true

  vault write pki/roles/webhook-service \
    allowed_domains=\"webhook.local,webhook-zt,svc,cluster.local\" \
    allow_subdomains=true \
    allow_any_name=true \
    max_ttl=\"24h\" \
    generate_lease=true

  vault read -field=certificate pki/cert/ca > /tmp/current_root_ca.crt
"

echo "Extracting Root CA and creating Kubernetes Secret"
mkdir -p certs
kubectl cp vault-0:/tmp/current_root_ca.crt ./certs/root_ca.crt

kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt=./certs/root_ca.crt \
  --dry-run=client --namespace=sentinel-data-plane -o yaml | kubectl apply -f -

base64 -w0 ./certs/root_ca.crt > ./certs/root_ca_base64.txt
echo "Root CA Base64 saved to ./certs/root_ca_base64.txt"

echo "PKI Engine configured successfully"
