# Notes — Control-Plane Vault Authentication Failure

## Problem

The control-plane could validate data-plane K8s tokens against the K8s API, but failed when trying to authenticate to Vault for certificate issuance:

```
Cannot login using Kubernetes: permission denied
```

Full stack trace pointed to `KubernetesAuthentication.login()` in Spring Vault — the control-plane's K8s service account token was being rejected by Vault.

## Root Cause

The `vault-configurer` service account (used by the configure-vault job) was missing the `system:auth-delegator` ClusterRoleBinding.

### How Vault K8s Auth Works

1. A client (control-plane) sends its K8s SA token to Vault's `/auth/kubernetes/login` endpoint
2. Vault needs to validate that token — it calls the K8s **TokenReview API** to ask K8s "is this token valid?"
3. To call the TokenReview API, Vault needs its own K8s token — this is the `token_reviewer_jwt` configured on the K8s auth backend
4. The configure job passed its own SA token (`vault-configurer`) as the `token_reviewer_jwt`
5. The `vault-configurer` SA had no permission to create `tokenreviews` → K8s returned 403 → Vault returned "permission denied"

### The Fix

Added a ClusterRoleBinding granting `vault-configurer` the built-in `system:auth-delegator` role:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-configurer-auth-delegator
subjects:
  - kind: ServiceAccount
    name: vault-configurer
    namespace: default
roleRef:
  kind: ClusterRole
  name: system:auth-delegator
  apiGroup: rbac.authorization.k8s.io
```

File: `infra/k8s/vault/configure/configure-vault-rbac.yaml`

After applying the RBAC, the configure job was re-run to re-write the `token_reviewer_jwt` into Vault's K8s auth config with the newly-privileged SA token.

## Secondary Issues Encountered

### 1. PostSync Hook Not Running

The configure-vault job was defined as a PostSync hook in the `vault` ArgoCD app, but it was in a **secondary source path** (`infra/k8s/vault/configure`). ArgoCD multi-source apps don't process hook annotations from secondary sources — the hook was silently ignored.

**Workaround**: Ran the job manually with `kubectl create -f`.

**Proper fix needed**: Either move the configure job into the primary Helm chart source as a Helm hook, or keep it as a separate ArgoCD app without hook annotations (just a regular Job that runs on apply).

### 2. Vault Stuck in Raft Mode After Config Change

Switched from dev mode to Raft storage in `vault-values.yaml`, but:
- The old `vault-config` ConfigMap persisted with Raft config even after reverting to dev mode
- The Helm chart only creates this ConfigMap when `mode != "dev"`, so it should have been deleted by ArgoCD's prune — but it wasn't
- Vault in Raft mode requires manual `operator init` and `operator unseal`, which we don't have automation for

**Resolution**: Reverted to dev mode, deleted the stale ConfigMap and StatefulSet, let ArgoCD recreate with `vault server -dev`.

### 3. Stale Vault Pod

After the StatefulSet was updated to dev mode, the running pod was still using the old Raft config. StatefulSet rolling updates don't always trigger pod recreation for config changes.

**Resolution**: Deleted the pod manually (`kubectl delete pod vault-0`) to force recreation with the new container args.

### 4. ArgoCD CLI Permission Issues

The ArgoCD CLI (`argocd app refresh`, `argocd app sync`) returned `PermissionDenied` errors for some commands. The `argocd app sync --force` command worked intermittently but often hit "another operation is already in progress" due to auto-sync running.

**Workaround**: Relied on ArgoCD's auto-sync policy and manual kubectl operations instead.

## Files Changed

| File | Change |
|------|--------|
| `infra/k8s/vault/vault-values.yaml` | Reverted from Raft to dev mode |
| `infra/k8s/vault/configure/configure-vault-rbac.yaml` | Added `system:auth-delegator` ClusterRoleBinding |
| `infra/k8s/vault/configure/configure-vault-job.yaml` | Made idempotent, improved readiness checks |
| `argocd/applications/vault.yaml` | Added configure directory as secondary source |
| `argocd/applications/vault-configure.yaml` | Deleted (merged into vault app) |

## Debugging Commands That Were Useful

```bash
# Check what Vault auth methods are enabled
kubectl exec -n default vault-0 -- vault auth list

# Check K8s auth config in Vault
kubectl exec -n default vault-0 -- vault read auth/kubernetes/config

# Check the K8s auth role
kubectl exec -n default vault-0 -- vault read auth/kubernetes/role/sentinel-role

# Test Vault login from inside the control-plane pod
kubectl exec -n sentinel-control-plane <pod> -- sh -c \
  'TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token) && \
   wget -qO- --post-data="{\"jwt\": \"$TOKEN\", \"role\": \"sentinel-role\"}" \
   http://vault-internal.default.svc.cluster.local:8200/v1/auth/kubernetes/login'

# Test if Vault can reach the K8s TokenReview API
kubectl exec -n default vault-0 -- sh -c \
  'wget --no-check-certificate -qO- \
   --post-data="{\"apiVersion\": \"authentication.k8s.io/v1\", \"kind\": \"TokenReview\", \"spec\": {\"token\": \"test\"}}" \
   https://kubernetes.default.svc/apis/authentication.k8s.io/v1/tokenreviews'

# Check Vault server logs
kubectl logs -n default vault-0 --tail=50

# Check if the configure job ran
kubectl get jobs -n default
kubectl logs -n default job/configure-vault
```

## Lessons

1. **`token_reviewer_jwt` needs `system:auth-delegator`** — any SA used as the token reviewer for Vault's K8s auth must have this built-in ClusterRole
2. **ArgoCD multi-source hooks are unreliable** — PostSync hooks only work reliably from the primary source
3. **Vault dev mode = in-memory** — any restart wipes all config. The configure job must re-run after every Vault restart
4. **Helm chart ConfigMap lifecycle** — when a Helm condition stops rendering a resource, ArgoCD prune should delete it, but stale ConfigMaps can persist if the StatefulSet references them
5. **Always delete stale pods after config changes** — StatefulSet updates don't always trigger pod recreation

---

## How to Fix It When It Happens Again (Exact Steps)

If you see `Cannot login using Kubernetes: permission denied` in the control-plane logs, follow these steps:

### 1. Ensure the RBAC binding exists
```bash
# Check if the auth-delegator binding is present
kubectl get clusterrolebinding vault-configurer-auth-delegator

# If missing or outdated, apply it:
kubectl apply -f infra/k8s/vault/configure/configure-vault-rbac.yaml
```

### 2. Re-run the configure job to refresh Vault's K8s auth config
```bash
# Delete any existing job (ignore if not found)
kubectl delete job configure-vault -n default --ignore-not-found

# Create and run the job
kubectl create -f infra/k8s/vault/configure/configure-vault-job.yaml

# Wait for it to complete (typically <1 minute)
sleep 30

# Verify it succeeded
kubectl logs -n default job/configure-vault
# Look for: "Vault configuration complete" at the end
```

### 3. Test that the fix works
```bash
# Get the control-plane pod name
POD=$(kubectl get pod -n sentinel-control-plane -l app.kubernetes.io/name=control-plane -o jsonpath='{.items[0].metadata.name}')

# Test Vault login from inside the pod
kubectl exec -n sentinel-control-plane $POD -- sh -c \
  'TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token) && \
   wget -qO- --post-data="{\"jwt\": \"'$TOKEN'\", \"role\": \"sentinel-role\"}" \
   http://vault-internal.default.svc.cluster.local:8200/v1/auth/kubernetes/login'
```

**Success** looks like:
```json
{
  "request_id": "...",
  "lease_id": "",
  "renewable": false,
  "lease_duration": 0,
  "data": null,
  "wrap_info": null,
  "warnings": null,
  "auth": {
    "client_token": "hvs.CAESIC...",
    "accessor": "...",
    ...
  }
}
```

**Failure** (still broken) looks like:
```
wget: server returned error: HTTP/1.1 403 Forbidden
```

### 4. If it still fails, repeat from step 1
Sometimes Vault restarts between steps — just repeat the process. The job is fast and safe to re-run.

> **Tip**: Since Vault runs in dev mode (in-memory), its configuration is lost on every restart. The configure job must re-run after each Vault pod restart. Consider automating this with a ArgoCD Application that has `selfHeal: true` on the configure job, or switch Vault to production mode with persistent storage if you need HA.
