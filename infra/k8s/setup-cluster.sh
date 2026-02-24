#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"

if ! command -v kind &>/dev/null; then
  echo "Kind not found. Installing Kind..."
  # For AMD64 / x86_64
  [ $(uname -m) = x86_64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.31.0/kind-linux-amd64
  # For ARM64
  [ $(uname -m) = aarch64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.31.0/kind-linux-arm64
  chmod +x ./kind
  sudo mv ./kind /usr/local/bin/kind
fi

if ! kind get clusters | grep -q "^${CLUSTER_NAME}\$"; then
  echo "Creating kind cluster: ${CLUSTER_NAME}"
  kind create cluster --name "${CLUSTER_NAME}"
else
  echo "Cluster ${CLUSTER_NAME} already exists."
fi

echo "Loading Data Plane image into kind"
kind load docker-image "sentinel-data-plane:latest" --name "${CLUSTER_NAME}"

echo "Loading Control Plane image into kind"
kind load docker-image "sentinel-control-plane:latest" --name "${CLUSTER_NAME}"

echo "Cluster setup complete"
kubectl cluster-info
