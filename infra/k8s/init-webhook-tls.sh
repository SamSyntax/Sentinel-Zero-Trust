#!/usr/bin/env bash
set -e

echo "Issuing webhook certificate from Vault..."
VAULT_TOKEN=$(jq -r '.root_token' ./certs/cluster-keys.json)
kubectl exec vault-0 -- sh -c "
  export VAULT_TOKEN=${VAULT_TOKEN}
  export VAULT_ADDR=http://127.0.0.1:8200
  vault read -field=certificate pki/cert/ca > /tmp/current_root_ca.crt
  vault write -format=json pki/issue/webhook-service \
    common_name=\"sentinel-control-plane.sentinel-control-plane.svc\" \
    ttl=\"24h\" > /tmp/webhook_cert.json
"

echo "Extracting certificate and key..."
mkdir -p certs
kubectl cp vault-0:/tmp/webhook_cert.json ./certs/webhook_cert.json
kubectl cp vault-0:/tmp/current_root_ca.crt ./certs/root_ca.crt

python3 -c "
import json
with open('./certs/webhook_cert.json') as f:
    data = json.load(f)
with open('./certs/tls.crt', 'w') as f:
    f.write(data['data']['certificate'].rstrip() + '\n')
    issuing_ca = data['data'].get('issuing_ca', '').strip()
    if issuing_ca:
        f.write(issuing_ca + '\n')
with open('./certs/tls.key', 'w') as f:
    f.write(data['data']['private_key'])
with open('./certs/issuing_ca.crt', 'w') as f:
    f.write(data['data']['issuing_ca'])
"

base64 -w0 ./certs/issuing_ca.crt > ./certs/issuing_ca_base64.txt

echo "Creating Kubernetes TLS Secret..."
kubectl create namespace sentinel-control-plane --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret tls webhook-server-tls \
  --cert=./certs/tls.crt \
  --key=./certs/tls.key \
  --dry-run=client --namespace=sentinel-control-plane -o yaml | kubectl apply -f -

echo "Creating webhook CA secret for MutatingWebhookConfiguration..."
kubectl create secret generic webhook-ca \
  --from-file=root_ca.crt=./certs/root_ca.crt \
  --dry-run=client --namespace=sentinel-control-plane -o yaml | kubectl apply -f -

echo "Creating sentinel-root-ca secret for data plane proxies..."
kubectl create secret generic sentinel-root-ca \
  --from-file=root_ca.crt=./certs/root_ca.crt \
  --namespace=default --dry-run=client -o yaml | kubectl apply -f -

echo "Webhook TLS Secret created successfully."
