#!/usr/bin/env bash

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$DIR/../../.."
KEYS_FILE="$PROJECT_ROOT/certs/cluster-keys.json"

TOKEN=$(jq -r '.root_token' $KEYS_FILE)
echo "{\"token\": \"$TOKEN\"}"
