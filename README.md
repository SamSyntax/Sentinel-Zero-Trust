**Sentinel ZT-Proxy** is a Zero Trust Architecture (ZTA) implementation enforcing Mutual TLS (mTLS) micro-segmentation between services.

- **Data Plane (PEP):** Go-based high-performance reverse proxy. Handles TLS termination, Hitless Certificate Rotation, and request forwarding.
- **Control Plane (PDP):** Java 21 / Spring Boot 3 application. Manages service identities and interacts with the PKI.
- **Infrastructure (Trust Engine):** HashiCorp Vault (PKI Engine) and Open Policy Agent (OPA) orchestrated via Terraform, Kubernetes and bash bootstrap scripts. Ran and tested locally using [Kind](https://kind.sigs.k8s.io/).

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
