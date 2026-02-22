#!/bin/bash

set -euxo pipefail

docker exec -e VAULT_TOKEN=root sentinel-vault vault write -format=json \
  pki/issue/sentinel-service common_name="proxy.sentinel.local" >certs/proxy-bundle.json

docker exec -e VAULT_TOKEN=root sentinel-vault vault write -format=json \
  pki/issue/sentinel-service common_name="client.sentinel.local" >certs/client-bundle.json

python3 -c "import sys, json; d=json.load(sys.stdin)['data']; open('certs/client.crt','w').write(d['certificate']); open('certs/client.key','w').write(d['private_key'])" <certs/client-bundle.json
