#!/usr/bin/env bash

DIR=$(dirname "$0")

kubectl create configmap sentinel-zt-dashboard \
  --from-file="$DIR/dashboard-main.json" \
  --from-file="$DIR/kubernetes-dashboard.json" \
  --namespace observability \
  --dry-run=client -o yaml |
  kubectl label --local -f - grafana_dashboard=1 --dry-run=client -o yaml |
  kubectl apply -f -
