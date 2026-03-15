**Sentinel ZT-Proxy** is a Zero Trust Architecture (ZTA) implementation enforcing Mutual TLS (mTLS) micro-segmentation between services.

- **Data Plane (PEP):** Go-based high-performance reverse proxy. Handles TLS termination, Hitless Certificate Rotation, and request forwarding.
- **Control Plane (PDP):** Java 21 / Spring Boot 3 application. Manages service identities and interacts with the PKI.
- **Infrastructure (Trust Engine):** HashiCorp Vault (PKI Engine) and Open Policy Agent (OPA) orchestrated via Docker Compose.

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
- [ ] **Webhook TLS Bootstrapping** :
  - [ ] Secure injector with K8s CSR or static certs to satisfy HTTPS requirement.
- [ ] **Automated Sidecar Injection**:
  - [ ] Implement Mutating Admission Webhook in Java with idempotency checks.
  - [ ] Define injection logic for \`sentinel-proxy\`, \`sentinel-init\`, and volumes.
  - [ ] Create K8s \`MutatingWebhookConfiguration\` with namespace exclusions.
- [x] **Transparent Redirection** :
  - [x] Implement \`iptables\` logic in init-container for traffic interception.

### Phase 2: Production Readiness & Resiliency
- [ ] **Safety & Failure Policy** :
  - [ ] Define Fail-Open/Fail-Closed behavior for the injector.
- [ ] **Graceful Shutdown**:
  - [ ] Handle SIGTERM/SIGINT signals; implement \`server.Shutdown\`.
- [ ] **Health Probes & Exclusions** :
  - [ ] Implement \`/healthz\` and \`iptables\` bypass for Kubelet probes.
- [ ] **Advanced Load Balancing**:
  - [ ] Implement Endpoint Slices watcher and dynamic LB algorithms (Least-Request).
- [ ] **Retries & Circuit Breaking**:
  - [ ] Implement middleware for exponential backoff and state management.

### Phase 3: Observability & Policy
- [ ] **Distributed Tracing (OpenTelemetry)**:
  - [ ] Instrument Go proxy with OTel; propagate W3C TraceParent headers.
- [ ] **Prometheus Metrics**:
  - [ ] Export R.E.D metrics and cert expiry gauges.
- [ ] **Deep Policy Enforcement (OPA)**:
  - [ ] Implement sidecar-to-OPA interface; define Rego RBAC/ABAC policies.
- [ ] **Access Audit Logs**:
  - [ ] Enrich JSON logs with TLS metadata; implement sampling.

### Phase 4: Enterprise Scale
- [ ] **Intermediate CA Architecture**:
  - [ ] Automate Intermediate CA signing; update proxy to trust full chain.
- [ ] **Dynamic Configuration (xDS-like)**:
  - [ ] Implement gRPC streaming API for real-time config delivery.
- [ ] **HA Control Plane**:
  - [ ] Implement K8s Lease-based leader election for the Java Control Plane.
