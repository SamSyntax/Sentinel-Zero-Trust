#!/usr/bin/env bash

DIR=$(dirname "$0")

echo "Adding Grafana Helm Repository"
helm repo add grafana https://grafana.github.io/helm-charts 2>/dev/null
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null
helm repo update 2>/dev/null

set -e
echo "Cleaning previous installs..."
helm uninstall loki promtail grafana prometheus -n observability 2>/dev/null || true

echo "Installing Prometheus"
helm upgrade --install prometheus prometheus-community/prometheus \
  --namespace observability \
  --create-namespace \
  --set server.persistentVolume.enabled=true

echo "Installing Loki (SingleBinary)"
helm upgrade --install loki grafana/loki \
  --namespace observability \
  --values "$DIR/loki-values.yaml"

echo "Installing Promtail"
helm upgrade --install promtail grafana/promtail \
  --namespace observability \
  --set "config.clients[0].url=http://loki.observability.svc.cluster.local:3100/loki/api/v1/push"

echo "Installing Grafana"
helm upgrade --install grafana grafana/grafana \
  --namespace observability \
  --values "$DIR/grafana-values.yaml"

echo "Applying Dashboard ConfigMap"
$DIR/refresh-dashboard.sh

echo "Waiting for Pods..."
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=prometheus -n observability --timeout=120s
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=loki -n observability --timeout=120s
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=grafana -n observability --timeout=60s
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=promtail -n observability --timeout=60s

echo ""
echo "Observability Stack Deployed"
echo ""
echo "Verify DNS & connectivity:"
echo "  POD=\$(kubectl get pods -n observability -l app.kubernetes.io/name=grafana -o jsonpath='{.items[0].metadata.name}')"
echo "  kubectl exec -it \$POD -n observability -- curl http://loki.observability.svc.cluster.local:3100/loki/ready"
echo ""
echo "Access Grafana:"
echo "  kubectl port-forward svc/grafana 3000:80 -n observability"
echo "  → http://localhost:3000 (admin/admin)"
