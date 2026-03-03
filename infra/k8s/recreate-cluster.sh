#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
DIR=$(dirname "$0")

echo "Deleting Kind Cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}"

$DIR/kind/setup-cluster.sh
kubectl create namespace sentinel-data-plane --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace sentinel-control-plane --dry-run=client -o yaml | kubectl apply -f -
$DIR/vault/boot-vault.sh -a -p 8210
$DIR/observability/setup-observability.sh

helm upgrade --install control-plane $DIR/control-plane \
  -n sentinel-control-plane --create-namespace \
  --wait

helm upgrade --install data-plane $DIR/data-plane \
  -n sentinel-data-plane --create-namespace \
  --set service.type=NodePort --set service.nodePort=30443
