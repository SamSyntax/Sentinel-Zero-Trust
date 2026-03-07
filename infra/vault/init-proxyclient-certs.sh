#!/bin/bash
set -euxo pipefail

DIR=$(dirname "$0")
CERTS_DIR="$DIR/../../certs"
PROXY_BUNDLE="$CERTS_DIR/proxy-docker-bundle.json"
CLIENT_BUNDLE="$CERTS_DIR/client-docker-bundle.json"

proxy=$(docker exec -e VAULT_TOKEN=root sentinel-vault vault write -format=json \
  pki/issue/sentinel-service common_name="proxy.sentinel.local")
echo "$proxy" > "$PROXY_BUNDLE"

client=$(docker exec -e VAULT_TOKEN=root sentinel-vault vault write -format=json \
  pki/issue/sentinel-service common_name="client.sentinel.local")
echo "$client" > "$CLIENT_BUNDLE"

jq -r '.data.certificate' "$CLIENT_BUNDLE" > "$CERTS_DIR/docker-client.crt"
jq -r '.data.private_key' "$CLIENT_BUNDLE" > "$CERTS_DIR/docker-client.key"

echo "Certificates extracted successfully."
