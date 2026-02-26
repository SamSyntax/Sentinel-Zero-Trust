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

provider "vault" {
  address = "http://127.0.0.1:8200"
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
  backend          = vault_mount.pki.path
  name             = "sentinel-service"
  ttl              = "259200"
  allow_ip_sans    = true
  key_type         = "rsa"
  key_bits         = 2048
  allowed_domains  = ["sentinel.local", "sentinel.zt"]
  allow_subdomains = true
  generate_lease   = true
}

provider "kubernetes" {
  config_path = "~/.kube/config"
}

resource "kubernetes_namespace_v1" "sentinel_data_plane" {
  metadata {
    name = "sentinel-data-plane"
  }
}

resource "kubernetes_secret_v1" "sentinel_root_ca" {
  metadata {
    name      = "sentinel-root-ca"
    namespace = kubernetes_namespace_v1.sentinel_data_plane.metadata[0].name
  }
  data = {
    "root_ca.crt" = vault_pki_secret_backend_root_cert.sentinel_root.certificate
  }
}
