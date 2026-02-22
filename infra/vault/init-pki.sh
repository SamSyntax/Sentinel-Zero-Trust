#!/bin/bash

set -euxo pipefail

VAULT_ADDR='http://127.0.0.1:8200'
VAULT_TOKEN='root'
ROOT_CERT_PATH="$HOME/Documents/FinalProject/sentinel-zt/certs/root_ca.crt"

echo "--- Init PKI Secrets Engine ---"
docker exec -e VAULT_TOKEN=$VAULT_TOKEN sentinel-vault vault secrets enable pki || echo "PKI already enabled"
docker exec -e VAULT_TOKEN=$VAULT_TOKEN sentinel-vault vault secrets tune -max-lease-ttl=87600h pki
docker exec -e VAULT_TOKEN=$VAULT_TOKEN sentinel-vault vault write -field=certificate pki/root/generate/internal common_name="Sentinel Root CA" ttl=87600h >$ROOT_CERT_PATH
docker exec -e VAULT_TOKEN=$VAULT_TOKEN sentinel-vault vault write pki/roles/sentinel-service allowed_domains="sentinel.local" allow_subdomains=true max_ttl="72h" generate_lease=true

echo "--- PKI Setup Complete. Root CA saved to $ROOT_CERT_PATH  ---"
