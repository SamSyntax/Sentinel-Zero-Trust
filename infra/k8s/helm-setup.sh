#!/usr/bin/env bash

set -eo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NEUTRAL='\033[0;14m'
NC='\033[0m'

if ! command -v helm &>/dev/null; then
  echo -e "${GREEN}Helm not found. Installing Helm..."
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
  echo -e "${GREEN}Helm installed successfully.${NC}"
fi

echo -e "${GREEN}Adding Hashicorp Helm repo...${NC}"
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo udpate
helm upgrade --install vault hashicorp/vault \
  --set "server.dev.enabled=true" \
  --set "server.dev.devRootToken=root" \
  --set "server.extraArgs=-dev-listen-address=0.0.0.0:8200" \
  --wait
