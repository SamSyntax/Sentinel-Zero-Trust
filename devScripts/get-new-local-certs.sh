#!/usr/bin/env bash

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERT_DIR="$DIR/../certs"
mkdir -p "$CERT_DIR"
set -e

kubectl port-forward svc/sentinel-control-plane -n sentinel-control-plane 8081:8081 >/dev/null 2>&1 &
PF_PID=$!

echo "$CERT_DIR"

trap "kill $PF_PID" EXIT

for i in {1..10}; do
  if nc -z localhost 8081; then break; fi
  sleep 1
done
TOKEN=$(kubectl create token sentinel-data-plane -n sentinel-data-plane)

RESPONSE=$(curl -s -X POST http://localhost:8081/api/v1/identity/issue \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"serviceName": "data-plane"}')

if ! echo "$RESPONSE" | jq -e '.certificate' >/dev/null; then
  echo "Error: Invalid response from Control Plane"
  echo "$RESPONSE"
  exit 1
fi

echo "$RESPONSE" | jq -r '.certificate' >"$CERT_DIR/client.crt"
echo "$RESPONSE" | jq -r '.privateKey' >"$CERT_DIR/client.key"
echo "$RESPONSE" | jq -r '.issuingCa' >"$CERT_DIR/root_ca.crt"

echo "Success! Certificates saved to $CERT_DIR"
ls -l "$CERT_DIR"
