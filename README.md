# Zero Trust Architecture (ZTA) 

Kubernetes defaults to a perimeter-based security model where anything inside the network can freely talk to anything else, turning internal service-to-service communication into a trust-by-default blind spot. To solve this, I built a lightweight service mesh that replaces implicit network trust with cryptographic, per-service identity. Using mutating admission webhooks, a sidecar proxy is automatically injected into every pod at creation time, intercepting traffic to enforce mutual TLS without requiring changes to application code.

Each service receives its own verifiable certificate, preventing pods from impersonating one another and ensuring all internal communication is fully encrypted in transit. By moving away from manual log wrangling and open network access, this architecture hardens microservice communication at the transport layer while automating the operational overhead typically required to secure a cluster.

- **Data Plane (PEP):** Go-based high-performance reverse proxy. Handles TLS termination, Hitless Certificate Rotation, and request forwarding.
- **Control Plane (PDP):** Java 21 / Spring Boot 3 application. Manages service identities and interacts with the PKI.
- **Infrastructure (Trust Engine):** HashiCorp Vault (PKI Engine) orchestrated via Terraform, Kubernetes and bash bootstrap scripts. Ran and tested locally using [Kind](https://kind.sigs.k8s.io/).

## Motivation

I created this project for my Bachelor of Engineering degree. I wanted to combine security, programming and DevOps fields, my main goal was to dive deeper into Kubernetes operators, infrastructre and its API, networking, clustering and Kubernetes security. 

## Quick Start

```bash
git clone https://github.com/SamSyntax/Sentinel-Zero-Trust
cd Sentinel-Zero-Trust
```
```bash
make recreate-cluster
```

## Contributing

### Clone the repo

```bash
git clone https://github.com/SamSyntax/Sentinel-Zero-Trust
cd Sentinel-Zero-Trust
```

### Deploy into a local cluster (make sure that GNU Make is installed)

```bash
make recreate-cluster
```

## Usage

Kubernetes manifests are placed in the infra directory, most of the actual configuration is set there. This project is meant to be ran locally within a kind cluster.


### Submit a pull request

If you'd like to contribute, please fork the repository and open a pull request to the `master` branch.

### Phase 1: Core Zero Trust Identity & Security
- [x] **Identity Bootstrapping**:
  - [x] Integrate Java Control Plane with Vault PKI.
  - [x] Implement certificate issuance endpoint (\`/api/v1/identity/issue\`).
- [x] **Kubernetes Token Review**:
  - [x] Integrate Control Plane with Fabric8 Kubernetes SDK.
  - [x] Implement token validation service for ServiceAccount tokens.
- [x] **mTLS Enforcement**:
  - [x] Configure Go proxy with \`tls.RequireAndVerifyClientCert\`.
  - [x] Implement Root CA certificate verification.
- [x] **Secured Identity Issuance**:
  - [x] Implement \`SentinelSecurityInterceptor\` to extract identity from token.
  - [x] Enforce identity binding in \`IdentityController\` (prevent spoofing).
- [x] **Hitless Cert Rotation**:
  - [x] Implement background rotation goroutine in Go proxy.
  - [x] Atomic swap of \`tls.Certificate\` without connection drops.
- [x] **Webhook TLS Bootstrapping** :
  - [x] Secure injector with K8s CSR or static certs to satisfy HTTPS requirement.
- [x] **Automated Sidecar Injection**:
  - [x] Implement Mutating Admission Webhook in Java with idempotency checks.
  - [x] Define injection logic for \`sentinel-proxy\`, \`sentinel-init\`, and volumes.
  - [x] Create K8s \`MutatingWebhookConfiguration\` with namespace exclusions.
- [x] **Traffic redirection** :
  - [x] Implement \`iptables\` logic in init-container for traffic interception.

### Phase 2: Production Readiness & Resiliency
- [x] **Safety & Failure Policy** :
  - [ ] Define Fail-Open/Fail-Closed behavior for the injector.
- [x] **Graceful Shutdown**:
  - [x] Handle SIGTERM/SIGINT signals; implement \`server.Shutdown\`.
- [x] **Health Probes & Exclusions** :
  - [x] Implement \`/healthz\` and \`iptables\` bypass for Kubelet probes.
- [ ] **Advanced Load Balancing**:
  - [ ] Implement Endpoint Slices watcher and dynamic LB algorithms (Least-Request).
- [ ] **Retries & Circuit Breaking**:
  - [ ] Implement middleware for exponential backoff and state management.
