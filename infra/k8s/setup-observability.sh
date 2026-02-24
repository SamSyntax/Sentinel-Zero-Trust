#!/usr/bin/env bash
set -e

echo "Adding Grafana Helm Repository"
helm repo add grafana http://grafana.github.io/helm-charts
helm repo update

echo "Installing PLG Stack (Promtail, Loki, Grafana)"
helm install plg-stack grafana/loki-stack \
  --namespace observability \
  --create-namespace \
  --set grafana.enabled=true \
  --set promtail.enabled=true \
  --set prometheus.enabled=true \
  --wait

echo "observability Stack Deployed"
