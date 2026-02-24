#!/usr/bin/env bash

set -e

CLUSTER_NAME="sentinel-zt"
DIR=$(dirname "$0")

echo "Deleting Kind Cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}"

$DIR/kind/setup-cluster.sh
$DIR/helm-setup.sh
$DIR/observability/setup-observability.sh
$DIR/cluster-pki-init.sh

kubectl apply -f $DIR/control-plane/control-plane.yaml
kubectl apply -f $DIR/data-plane/data-plane.yaml
