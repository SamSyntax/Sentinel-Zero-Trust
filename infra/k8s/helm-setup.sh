#!/usr/bin/env bash

set -e

if ! command -v helm &>/dev/null; then
  echo "Helm not found. Installing Helm..."
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi

echo "Installing Hashicorp Vault Helm Chart"
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo udpate

helm install vault hashicorp/vault \
  --set "server.dev.enabled=true" \
  --set "server.dev.devRootToken=root" \
  --wait
