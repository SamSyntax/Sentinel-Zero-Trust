# AI Agent Instructions: Sentinel ZT-Proxy

## 1. Project Architecture & Context

**Sentinel ZT-Proxy** is a Zero Trust Architecture (ZTA) implementation enforcing Mutual TLS (mTLS) micro-segmentation between services.

- **Data Plane (PEP):** Go-based high-performance reverse proxy. Handles TLS termination, Hitless Certificate Rotation, and request forwarding.
- **Control Plane (PDP):** Java 21 / Spring Boot 3 application. Manages service identities and interacts with the PKI.
- **Infrastructure (Trust Engine):** HashiCorp Vault (PKI Engine) and Open Policy Agent (OPA) orchestrated via Docker Compose.

## 2. Global Development Standards

All AI agents must strictly adhere to the following engineering principles:

- **Code Quality:** Write clean, maintainable, and idiomatic code adhering to SOLID and DRY principles.
- **Formatting:** Enforce Prettier standards with a strict 80-character print width limit across all languages.
- **Documentation:** Use descriptive variable names. Provide brief, meaningful comments exclusively for complex, non-obvious logic.
- **CLI Snippets:** Provide copy-pasteable shell commands in fenced bash blocks. **Never** use a leading \`$\` prompt.
- **Code Modifications:** When providing updates to existing files, use fenced code blocks with the \`diff\` language, utilizing \`+\` and \`-\` markers for exact line replacements.
- **Troubleshooting:** Perform a structured Root Cause Analysis (RCA) for any errors. Explain _why_ the fix works at a system level, not just what the fix is.
- **Ambiguity:** If a request lacks sufficient context, halt execution and ask clarifying questions before generating large code blocks.

## 3. Specialized Agent Personas

### Persona: Go Systems Engineer (Data Plane)

- **Scope:** \`data-plane/\` directory.
- **Core Competencies:** \`net/http\`, \`crypto/tls\`, concurrency (goroutines/channels), and \`httputil.ReverseProxy\`.
- **Directives:** Prioritize performance and memory safety. Ensure all cryptographic operations handle edge cases (e.g., certificate expiration, missing SANs).

### Persona: Enterprise Java Architect (Control Plane)

- **Scope:** \`control-plane/\` directory.
- **Core Competencies:** Java 21 (Virtual Threads), Spring Boot 3.x, Spring Cloud Vault, and RESTful API design.
- **Directives:** Maintain strict separation of concerns (Controllers, Services, DTOs). Utilize Lombok to reduce boilerplate. Ensure all external API calls (to Vault or OPA) handle timeouts and retries resiliently.

### Persona: DevOps & Security Architect (Infrastructure)

- **Scope:** \`infra/\` directory, Docker, Vault CLI, Rego (OPA).
- **Core Competencies:** Infrastructure as Code, Container Networking, PKI lifecycle management.
- **Directives:** Prioritize security best practices (e.g., least privilege, non-root users). Ensure initialization scripts are idempotent and fail-safe (\`set -e\`). Suggest observability hooks (Prometheus/Grafana) for new infrastructure components.

## 4. Technical Architecture Overview

### Network Topology & Ports

- **:8443** - Go Data Plane (Inbound mTLS Proxy).
- **:8080** - Target Application (Unencrypted local traffic).
- **:8081** - Java Control Plane (REST API / Identity Provider).
- **:8200** - HashiCorp Vault (PKI Engine).
- **:8181** - Open Policy Agent (Rego Policy Engine).

### Request Lifecycle (The Zero Trust Flow)

1. **Identity Provisioning (Bootstrapping):** The Go Proxy initializes and sends an Identity Request to the Java Control Plane (`POST :8081/api/v1/identity/issue`). Java validates the request, queries Vault for a short-lived certificate, and returns the cryptographic bundle to Go.
2. **mTLS Handshake:** An external client initiates a TLS 1.3 connection to the Go Proxy (:8443). The Proxy strictly enforces client certificate validation (`tls.RequireAndVerifyClientCert`).
3. **Authorization (Policy Check):** The Proxy extracts the Client's Subject Alternative Name (SAN) from the certificate, pauses the request, and queries the Java Control Plane. Java delegates rule evaluation to OPA to determine if the specific route and method are permitted for that identity.
4. **Local Forwarding:** Upon successful authorization, the Go Proxy terminates the mTLS tunnel and proxies the raw HTTP request to the isolated Target Application (:8080) via loopback.
5. **Hitless Rotation:** A background goroutine inside the Go Proxy continuously monitors its own certificate's Time-To-Live (TTL). Shortly before expiration, it fetches a new certificate from the Control Plane and atomically swaps the `tls.Certificate` reference without dropping active TCP connections.

## 5. Production Readiness Roadmap

To transform Sentinel from a PoC to a production-ready service mesh for mid-size projects, the following enhancements are planned:

### A. Full Observability Stack

- **Distributed Tracing (OpenTelemetry):** Integrate OTel into the Go Data Plane to provide end-to-end request tracing. This allows for identifying bottlenecks and failures across complex service chains.
- **Prometheus Metrics:** Export real-time operational data from the proxy, including request rates (RPS), error percentages (5xx/4xx), p99 latency, and certificate health (TTL remaining).
- **Structured Access Logs:** Implement high-fidelity JSON logs that capture the verified identity (SAN), source IP, destination service, and TLS version for every request.

### B. Resilient Traffic Management

- **Retries & Timeouts:** Implement intelligent retry logic with exponential backoff and request timeouts in the Go proxy to handle transient network errors.
- **Circuit Breaking:** Add a circuit breaker pattern to prevent cascading failures by temporarily stopping requests to a service that is consistently failing.
- **Client-Side Load Balancing:** Enable the proxy to balance traffic between multiple upstream replicas, ensuring efficient resource utilization and high availability.

### C. Enterprise Security & Controls

- **Fine-Grained Authorization (OPA Deep Integration):** Move beyond simple mTLS by implementing Rego-based policies that inspect request paths, HTTP methods, and even payload data.
- **Intermediate CA Architecture:** Refactor the PKI integration to use an Intermediate CA signed by the Root CA in Vault. Leaf certificates should always be issued by the Intermediate CA to minimize Root CA exposure.
- **Dynamic Configuration:** Implement a dynamic config protocol (simplified xDS) to allow the Control Plane to push updates (like rotate TTLs or security policies) to proxies without service restarts.

### D. High Availability & Scalability

- **HA Control Plane:** Deploy the Java Control Plane in a multi-replica configuration with leader election (using Kubernetes Leases) to ensure zero-downtime identity issuance.
- **Advanced Kubernetes Probes:** Update Readiness probes to only report 'Ready' once the proxy has successfully bootstrapped its identity and established a healthy connection to both the Control Plane and its target application.

## 6. Implementation Checklist (TODO)

### Phase 1: Core Zero Trust Identity & Security

- [x] **Identity Bootstrapping**:
  - [x] Integrate Java Control Plane with Vault PKI.
  - [x] Implement certificate issuance endpoint (`/api/v1/identity/issue`).
- [x] **Kubernetes Token Review**:
  - [x] Integrate Control Plane with Fabric8 Kubernetes SDK.
  - [x] Implement token validation service for ServiceAccount tokens.
- [x] **mTLS Enforcement**:
  - [x] Configure Go proxy with `tls.RequireAndVerifyClientCert`.
  - [x] Implement Root CA certificate verification.
- [ ] **Secured Identity Issuance**:
  - [ ] Implement `SentinelSecurityInterceptor` to extract identity from token.
  - [ ] Enforce identity binding in `IdentityController` (prevent spoofing).
- [x] **Hitless Cert Rotation**:
  - [x] Implement background rotation goroutine in Go proxy.
  - [x] Atomic swap of `tls.Certificate` without connection drops.
- [ ] **Automated Sidecar Injection**:
  - [ ] Implement Mutating Admission Webhook in Java.
  - [ ] Define injection logic for `sentinel-proxy` container and volumes.
  - [ ] Create K8s `MutatingWebhookConfiguration` manifest.

### Phase 2: Production Readiness & Resiliency

- [ ] **Graceful Shutdown**:
  - [ ] Handle SIGTERM/SIGINT signals in Go data plane.
  - [ ] Implement `server.Shutdown` with proper context timeouts.
- [ ] **Basic Health Probes**:
  - [ ] Implement `/healthz` endpoint on administrative port.
- [ ] **Advanced Load Balancing**:
  - [ ] Implement K8s Endpoint Slices watcher for dynamic discovery.
  - [ ] Add Round-Robin and Least-Request balancing algorithms.
  - [ ] Implement weight-aware balancing based on pod labels.
- [ ] **Retries & Timeouts**:
  - [ ] Implement middleware for retry logic with exponential backoff.
  - [ ] Add support for per-route configurable timeouts.
  - [ ] Implement "retryable-status-codes" filter.
- [ ] **Circuit Breaking**:
  - [ ] Implement state management (Closed, Open, Half-Open).
  - [ ] Add error threshold tracking per upstream host.
  - [ ] Logic for automated circuit resetting after cooldown.

### Phase 3: Observability & Policy

- [ ] **Distributed Tracing (OpenTelemetry)**:
  - [ ] Instrument Go proxy with OTel SDK.
  - [ ] Implement B3 or W3C TraceParent header propagation.
  - [ ] Export traces to Jaeger or OTLP collector.
- [ ] **Prometheus Metrics**:
  - [ ] Export R.E.D metrics (Rate, Error, Duration).
  - [ ] Add certificate expiry age gauge.
  - [ ] Implement scraping endpoint (`/metrics`) on administrative port.
- [ ] **Deep Policy Enforcement (OPA)**:
  - [ ] Implement sidecar-to-OPA gRPC/HTTP interface in proxy.
  - [ ] Define standard Rego policies for RBAC/ABAC.
  - [ ] Sync verified identity metadata from Control Plane to OPA context.
- [ ] **Access Audit Logs**:
  - [ ] Enrich logs with TLS serial number and Subject identity.
  - [ ] Implement log sampling to reduce volume in high-traffic environments.

### Phase 4: Enterprise Scale

- [ ] **Intermediate CA Architecture**:
  - [ ] Automate Intermediate CA creation and signing in Vault.
  - [ ] Update proxy to trust the full cert chain.
  - [ ] Implement automated CRL/OCSP checking.
- [ ] **Dynamic Configuration (xDS-like)**:
  - [ ] Implement a gRPC streaming API in the Control Plane for config delivery.
  - [ ] Add config watcher and hot-reload logic in the Go proxy.
- [ ] **HA Control Plane**:
  - [ ] Implement Kubernetes Lease-based leader election.
  - [ ] Ensure cross-replica state consistency for certificate issuance logs.
