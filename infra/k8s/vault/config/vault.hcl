ui = true

listener "tcp" {
  address = "0.0.0.0:8200"
  tls_disable = "true"
}

storage "raft" {
  path = "/vault/data"
  node_id = "raft_node_1"
}

api_addr = "http://vault.default.svc.cluster.local:8200"
cluster_addr = "http://$(HOSTNAME).vault-internal:8201"
disbale_mlock = true
