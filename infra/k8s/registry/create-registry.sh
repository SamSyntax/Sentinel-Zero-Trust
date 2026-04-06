#!/usr/bin/env bash

set -e

REGISTRY_NAME="kind-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"

echo "Creating local Docker registry: ${REGISTRY_NAME} on port ${REGISTRY_PORT}..."

if docker inspect "${REGISTRY_NAME}" >/dev/null 2>&1; then
  echo "Registry already exists. Starting if stopped..."
  docker start "${REGISTRY_NAME}" 2>/dev/null || true
else
  docker run -d \
    --restart=always \
    -p "0.0.0.0:${REGISTRY_PORT}:5000" \
    --name "${REGISTRY_NAME}" \
    registry:2
fi

echo "Registry is running at localhost:${REGISTRY_PORT}"
