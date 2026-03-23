#!/usr/bin/env bash

NS=$1

if [[ -z "$NS" ]]; then
  NS=database
  echo "No namespace provided, defaulting to $NS"
fi

kubectl create namespace $NS

helm repo add bitnami https://charts.bitnami.com/bitnami

kubectl create secret generic postgres-credentials \
  --from-literal=postgres-password='postgres' \
  --from-literal=replication-password='postgres' \
  --namespace $NS

helm upgrade --install postgresql bitnami/postgresql \
  --namespace $NS \
  --create-namespace \
  --set auth.secretKeys.adminPasswordKey=postgres-password \
  --set auth.username=postgres \
  --set auth.existingSecret=postgres-credentials \
  --set architecture=replication \
  --set readReplicas.replicaCount=1 \
  --set pgbouncer.enabled=true \
  --set primary.resources.requests.cpu="100m" \
  --set primary.resources.requests.memory="256Mi" \
  --set primary.resources.limits.cpu="500m" \
  --set primary.resources.limits.memory="512Mi"

PSQL_PASS=$(kubectl get secret --namespace $NS postgresql -o jsonpath="{.data.postgres-password}")
DECODED_PASS=$(echo $PSQL_PASS | base64 -d)

kubectl port-forward svc/postgresql 5432:5432 -n $NS

echo "Postgres password: $PSQL_PASS"
echo "Decoded password: $DECODED_PASS"
