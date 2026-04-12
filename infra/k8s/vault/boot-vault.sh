#!/usr/bin/env bash
set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$DIR/../../.."
KEYS_FILE="$PROJECT_ROOT/certs/cluster-keys.json"
UNSEAL_KEY=""
FORWARD_PORT=8210

# Colour codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NEUTRAL='\033[0;14m'
NC='\033[0m'

clean_vault() {
  echo -e "${GREEN}Cleaning up old Vault deployment...${NC}"
  helm uninstall vault 2>/dev/null || true
  kubectl delete pvc data-vault-0 --ignore-not-found=true 2>/dev/null || true
}

install_vault() {
  echo -e "${GREEN}Installing Vault...${NC}"
  if ! output=$(helm upgrade --install vault hashicorp/vault -f "$DIR/vault-values.yaml" --wait 2>&1); then
    echo -e "${RED}Error installing Vault:${NC}"
    echo -e "${RED}$output${NC}"
    exit 1
  fi
}

create_config_map() {
  echo -e "${GREEN}Creating ConfigMap...${NC}"

  if ! output=$(kubectl create configmap vault-config --from-file=vault.hcl="$DIR/config/vault.hcl" -n default --dry-run=client -o yaml | kubectl apply -f - 2>&1); then
    echo -e "${RED}Error creating ConfigMap:${NC}\n${RED}$output${NC}"
    exit 1
  fi

  if ! output=$(kubectl annotate configmap vault-config meta.helm.sh/release-name=vault --overwrite 2>&1); then
    echo -e "${RED}Error annotating ConfigMap (release-name):${NC}\n${RED}$output${NC}"
    exit 1
  fi

  if ! output=$(kubectl annotate configmap vault-config meta.helm.sh/release-namespace=default --overwrite 2>&1); then
    echo -e "${RED}Error annotating ConfigMap (release-namespace):${NC}\n${RED}$output${NC}"
    exit 1
  fi

  if ! output=$(kubectl label configmap vault-config app.kubernetes.io/managed-by=Helm --overwrite 2>&1); then
    echo -e "${RED}Error labeling ConfigMap:${NC}\n${RED}$output${NC}"
    exit 1
  fi
}

get_unseal_key() {
  if init=$(kubectl exec vault-0 -- vault status -format=json | jq -e .initialized); then
    echo -e "${BLUE}Vault is already initialized.${NC}"
    return false
  fi
  echo -e "${BLUE}Initializing vault...${NC}"
  if ! output=$(kubectl exec vault-0 -- vault operator init -key-shares=1 -key-threshold=1 -format=json 2>&1); then
    echo -e "${RED}Error initializing Vault:${NC}\n${RED}$output"
    echo -e "${YELLOW} $output ${NC}"
    return false
  else
    echo "$output" >"$KEYS_FILE"
  fi
  UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0]' "$KEYS_FILE" | tr -d '[:space:]')
  return true
}

unseal_vault() {
  if ! sealed=$(kubectl exec vault-0 -- vault status --format=json | jq -e .sealed); then
    echo -e "${BLUE}Vault is already unsealed.${NC}"
    exit 0
  fi
  echo -e "${BLUE}Unsealing Vault...${NC}"
  init=get_unseal_key
  if [[ -z "$UNSEAL_KEY" || "$UNSEAL_KEY" == "null" ]]; then
    echo -e "${RED}Error: Failed to extract UNSEAL_KEY. Check your $KEYS_FILE${NC}"
    exit 1
  fi
  if ! output=$(kubectl exec vault-0 -- vault operator unseal "$UNSEAL_KEY" </dev/null 2>&1); then
    echo -e "${RED}Error unsealing Vault:${NC}\n${RED}$output${NC}"
    exit 1
  fi
  echo -e "${GREEN}Vault successfully unsealed.${NC}"
}

apply_terraform() {
  echo -e "${BLUE}Applying Terraform (PKI & Secrets)...${NC}"

  kubectl port-forward --address 127.0.0.1 pod/vault-0 $1:8200 >/dev/null 2>&1 &
  PF_PID=$!
  trap "kill $PF_PID 2>/dev/null || true" EXIT

  export VAULT_ADDR="http://127.0.0.1:$1"

  cd "$PROJECT_ROOT/infra/terraform"

  if ! output=$(terraform init 2>&1); then
    echo -e "${RED}Error during Terraform Init:${NC}\n${RED}$output${NC}"
    exit 1
  fi

  if ! output=$(terraform apply -var "VAULT_ADDR=$VAULT_ADDR" -auto-approve 2>&1); then
    echo -e "${RED}Error during Terraform Apply:${NC}\n${RED}$output${NC}"
    exit 1
  fi
}

sync_vault_token() {
  echo -e "${BLUE}Synchronizing Vault Token to Application Namespace...${NC}"
  APP_NS="sentinel-control-plane"

  if ! output=$(kubectl create namespace $APP_NS --dry-run=client -o yaml | kubectl apply -f - 2>&1); then
    echo -e "${RED}Error creating namespace $APP_NS:${NC}\n${RED}$output${NC}"
    exit 1
  fi

  ROOT_TOKEN=$(jq -r '.root_token' "$KEYS_FILE")

  if ! output=$(kubectl create secret generic control-plane-vault-conf --from-literal=vault-token="$ROOT_TOKEN" -n $APP_NS --dry-run=client -o yaml | kubectl apply -f - 2>&1); then
    echo -e "${RED}Error creating Vault Secret:${NC}\n${RED}$output${NC}"
    exit 1
  fi
}

print_help() {
  echo -e "${NC}\n"
  echo -e "${GREEN}Usage: $0 [OPTIONS]"
  echo -e "${NEUTRAL}"
  echo -e "Options:"
  echo -e "  -h, --help          Display this help message"
  echo -e "  -a, --all           Run full bootstrap (clean, install, unseal, terraform, sync)"
  echo -e "  -c, --clean         Only clean the vault"
  echo -e "  -i, --install       Only create configmap and install vault"
  echo -e "  -u, --unseal        Only unseal the vault"
  echo -e "  -t, --terraform     Only apply terraform"
  echo -e "  -s, --sync          Only sync the vault token to application namespace"
  echo -e "  -p, --port          Local port to forward vault to (default: 8200)"
  echo -e "${NC}"
}

# All args to be parsed
RUN_ALL=false
RUN_CLEAN=false
RUN_INSTALL=false
RUN_UNSEAL=false
RUN_TERRAFORM=false
RUN_SYNC=false

LOCAL_PORT=8200

if [[ $# -eq 0 ]]; then
  print_help
  exit 0
fi

while [[ $# -gt 0 ]]; do
  case $1 in
  -h | --help)
    print_help
    exit 0
    ;;
  -a | --all)
    RUN_ALL=true
    ;;
  -c | --clean)
    RUN_CLEAN=true
    ;;
  -i | --install)
    RUN_INSTALL=true
    ;;
  -u | --unseal)
    RUN_UNSEAL=true
    ;;
  -t | --terraform)
    RUN_TERRAFORM=true
    ;;
  -s | --sync)
    RUN_SYNC=true
    ;;
  -p | --port)
    LOCAL_PORT=$2
    shift
    ;;
  *)
    echo -e "Unknown option: $1"
    print_help
    exit 1
    ;;
  esac
  shift
done

if [[ "$RUN_ALL" == true ]]; then
  RUN_CLEAN=true
  RUN_INSTALL=true
  RUN_UNSEAL=true
  RUN_TERRAFORM=true
  RUN_SYNC=true
fi

if [[ "$RUN_CLEAN" == true ]]; then
  clean_vault || true
fi

if [[ "$RUN_INSTALL" == true ]]; then
  create_config_map
  install_vault
  echo -e "${BLUE}Waiting for pod to be ready...${NC}"
  kubectl wait --for=jsonpath='{.status.phase}'=Running pod/vault-0 --timeout=60s
fi

if [[ "$RUN_UNSEAL" == true ]]; then
  unseal_vault
fi

if [[ "$RUN_TERRAFORM" == true ]]; then
  apply_terraform $LOCAL_PORT
fi

if [[ "$RUN_SYNC" == true ]]; then
  # sync_vault_token
  echo "Skipping sync_vault_token"
fi

echo -e "${GREEN}Done!${NC}"
