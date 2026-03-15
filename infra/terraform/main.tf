terraform {
  required_providers {
    vault = {
      source  = "hashicorp/vault"
      version = "~> 4.0"
    }
  }
}

data "external" "vault_keys" {
  program = ["bash", "${path.module}/helper-scripts/get-vault-token.sh"]
}

variable "VAULT_ADDR" {
  type    = string
  default = "http://127.0.0.1:8200"
}

provider "vault" {
  address = var.VAULT_ADDR
  token   = data.external.vault_keys.result.token

  skip_child_token = true
}

resource "vault_mount" "pki" {
  path                      = "pki"
  type                      = "pki"
  default_lease_ttl_seconds = 3600
  max_lease_ttl_seconds     = 31536000
}

resource "vault_pki_secret_backend_root_cert" "sentinel_root" {
  backend            = vault_mount.pki.path
  type               = "internal"
  common_name        = "Sentinel Root CA"
  ttl                = "31536000"
  format             = "pem"
  private_key_format = "der"
  key_type           = "rsa"
  key_bits           = 2048
}

resource "vault_pki_secret_backend_role" "sentinel_role" {
  backend       = vault_mount.pki.path
  name          = "sentinel-service"
  ttl           = "259200"
  allow_ip_sans = true
  key_type      = "rsa"
  key_bits      = 2048
  allowed_uri_sans = ["spiffe://*"]
  allow_any_name   = true
  generate_lease   = true
}

provider "kubernetes" {
  config_path = "~/.kube/config"
}

resource "kubernetes_secret_v1" "sentinel_root_ca" {
  metadata {
    name      = "sentinel-root-ca"
    namespace = "sentinel-data-plane"
  }
  data = {
    "root_ca.crt" = vault_pki_secret_backend_root_cert.sentinel_root.certificate
  }
}

resource "vault_auth_backend" "kubernetes" {
  type = "kubernetes"
}

resource "vault_kubernetes_auth_backend_config" "k8s" {
  backend         = vault_auth_backend.kubernetes.path
  kubernetes_host = "https://kubernetes.default.svc.cluster.local:443"
}

resource "vault_policy" "sentinel_policy" {
  name   = "sentinel-app-policy"
  policy = <<EOT
    path "pki/issue/sentinel-service" {
      capabilities = ["create", "update"]
  }
  EOT
}

resource "vault_kubernetes_auth_backend_role" "sentinel_role" {
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "sentinel-role"
  bound_service_account_names      = ["sentinel-control-plane"]
  bound_service_account_namespaces = ["sentinel-control-plane"]
  token_policies                   = [vault_policy.sentinel_policy.name]
  token_ttl                        = 3600
}
