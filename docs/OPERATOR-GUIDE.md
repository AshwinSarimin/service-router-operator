# Operator Guide

This guide is for platform engineers responsible for deploying, configuring, and operating the Service Router Operator.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Monitoring & Observability](#monitoring--observability)
- [Upgrade Strategy](#upgrade-strategy)
- [Backup & Recovery](#backup--recovery)
- [Common Operational Tasks](#common-operational-tasks)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Components

| Component | Version | Purpose |
|-----------|---------|---------|
| Kubernetes | 1.24+ | Operator runtime |
| Istio | 1.18+ | Gateway resources |
| ExternalDNS | 0.13+ | DNS record provisioning |
| cert-manager (optional) | 1.11+ | TLS certificate management |

### ExternalDNS Setup

The operator requires ExternalDNS configured to watch CRDs:

```yaml
args:
  - --source=crd
  - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
  - --crd-source-kind=DNSEndpoint
  - --txt-owner-id=external-dns-{region}
  - --provider=azure-private-dns
```

See [ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md) for complete configuration.

### Istio Setup

An Istio Ingress Gateway (LoadBalancer Service) must exist before deploying Gateway CRDs:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: aks-istio-ingressgateway-internal
  namespace: istio-system
spec:
  type: LoadBalancer
```

## Installation

See [Installation Guide](INSTALLATION.md) for detailed procedures.

### Quick Install (Kustomize)

```bash
# Install CRDs
kubectl apply -k config/crd

# Install operator
kubectl apply -k config/default
```

### Verification

```bash
# Check operator pod
kubectl get pods -n service-router-system

# Check CRDs installed
kubectl get crds | grep router.io

# Check operator logs
kubectl logs -n service-router-system deployment/service-router-operator
```

## Configuration

### Operator Flags

```bash
--metrics-bind-address=:8080          # Prometheus metrics endpoint
--health-probe-bind-address=:8081     # Health probe endpoint
--leader-elect=true                   # Enable leader election for HA
--zap-log-level=info                  # Log level (debug, info, warn, error)
```

### Resource Limits

| Cluster Size | ServiceRoutes | CPU Request | Memory Request |
|--------------|---------------|-------------|----------------|
| Small | < 50 | 100m | 64Mi |
| Medium | 50-200 | 200m | 128Mi |
| Large | 200-500 | 500m | 256Mi |
| X-Large | 500+ | 1000m | 512Mi |

### High Availability

```yaml
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: manager
          args:
            - --leader-elect=true  # Required for HA
```

Leader election ensures only one replica reconciles at a time. Failover is automatic in under 30 seconds.

## Monitoring & Observability

### Prometheus Metrics

The operator exposes metrics on `:8080/metrics`. Key metrics:

```
# Reconciliation duration per controller
controller_runtime_reconcile_time_seconds{controller="serviceroute"}

# Reconciliation success/error rate
controller_runtime_reconcile_total{controller="serviceroute",result="success"}
controller_runtime_reconcile_total{controller="serviceroute",result="error"}

# Active custom resources
serviceroute_active_total
dnspolicy_active_total
gateway_active_total
```

Configure scraping with a `ServiceMonitor`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: service-router-operator
  namespace: service-router-system
spec:
  selector:
    matchLabels:
      app: service-router-operator
  endpoints:
    - port: metrics
      interval: 30s
```

### Logging

The operator uses structured JSON logging. Useful filters:

```bash
# All reconciliation errors
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.level == "error")'

# Logs for a specific resource
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.name == "api-route" and .namespace == "myapp")'
```

### Kubernetes Events

```bash
# Events for a specific ServiceRoute
kubectl describe serviceroute -n myapp api-route

# All operator-generated events
kubectl get events -A \
  --field-selector involvedObject.apiVersion=routing.router.io/v1alpha1
```

### Status Conditions

```bash
# Check ServiceRoute status
kubectl get serviceroute -n myapp api-route -o yaml | yq '.status'

# List all ServiceRoutes with Ready status
kubectl get serviceroutes -A -o custom-columns=\
NAME:.metadata.name,\
NAMESPACE:.metadata.namespace,\
READY:.status.conditions[?(@.type=="Ready")].status
```

## Upgrade Strategy

### Upgrade Order

CRDs must be upgraded **before** the operator:

```bash
# 1. Upgrade CRDs first
kubectl apply -f config/crd/bases/
kubectl wait --for condition=established --timeout=60s \
  crd/serviceroutes.routing.router.io

# 2. Upgrade operator
kubectl apply -k config/default
# or via Helm:
helm upgrade service-router-operator service-router/service-router-operator \
  --namespace service-router-system \
  --set image.tag=v1.1.0
```

### Rolling Upgrade (Zero Downtime)

```yaml
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
```

### Rollback

```bash
helm rollback service-router-operator -n service-router-system
```

> **Note**: CRD rollback is not supported. Fix forward if CRD changes cause issues.

## Backup & Recovery

### Backup Custom Resources

```bash
kubectl get clusteridentities -o yaml > backup/clusteridentities.yaml
kubectl get dnsconfigurations -o yaml > backup/dnsconfigurations.yaml
kubectl get gateways -A -o yaml > backup/gateways.yaml
kubectl get dnspolicies -A -o yaml > backup/dnspolicies.yaml
kubectl get serviceroutes -A -o yaml > backup/serviceroutes.yaml
```

### Restore

```bash
# 1. Ensure operator is running
kubectl get pods -n service-router-system

# 2. Restore resources
kubectl apply -f backup/crds.yaml       # if CRDs were lost
kubectl apply -f backup/clusteridentities.yaml
kubectl apply -f backup/dnsconfigurations.yaml
kubectl apply -f backup/gateways.yaml
kubectl apply -f backup/dnspolicies.yaml
kubectl apply -f backup/serviceroutes.yaml

# 3. Verify reconciliation
kubectl get serviceroutes -A
kubectl get dnsendpoints -A
```

### Disaster Recovery Notes

- **Operator pod deleted**: Automatic recovery (< 1 minute via Deployment controller)
- **CRDs accidentally deleted**: Cascading delete removes all custom resources. Restore CRDs, then restore resources from backup.
- **Full cluster loss**: Deploy new cluster, install prerequisites and operator, restore from backup. Expected RTO: 15-30 minutes.

## Common Operational Tasks

### Restart the Operator

```bash
kubectl rollout restart deployment/service-router-operator -n service-router-system
```

### Force Reconciliation

```bash
kubectl annotate serviceroute -n myapp api-route \
  reconcile.router.io/force="$(date +%s)"
```

### Clean Up a Namespace

When deleting a namespace with operator resources, clean up gracefully:

```bash
# Delete ServiceRoutes first (DNSEndpoints are deleted via OwnerReferences)
kubectl delete serviceroutes -n myapp --all

# Wait for DNSEndpoints to be removed
kubectl wait --for=delete dnsendpoint -n myapp --all --timeout=60s

# Delete namespace
kubectl delete namespace myapp
```

### Clean Up Stale DNS Records

If DNSEndpoints are deleted but records remain in Azure Private DNS:

```bash
# Check for orphaned records
az network private-dns record-set cname list \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  | jq '.[] | select(.fqdn | contains("myapp"))'
```

**Prevention**: Run ExternalDNS with `--policy=sync`.

## Troubleshooting

### Operator Not Starting

Common causes:

```bash
# CRDs not installed
kubectl get crd | grep router.io

# RBAC permissions missing
kubectl auth can-i get clusteridentities \
  --as=system:serviceaccount:service-router-system:service-router-operator

# Image pull failure
kubectl describe pod -n service-router-system -l app=service-router-operator
```

### ServiceRoute Not Creating DNSEndpoints

```bash
# 1. Check ServiceRoute conditions
kubectl get serviceroute -n myapp api-route \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'

# 2. Check DNSPolicy is active
kubectl get dnspolicy -n myapp -o yaml | yq '.status'

# 3. Check Gateway exists
kubectl get gateway -n istio-system

# 4. Check ClusterIdentity exists
kubectl get clusteridentity

# 5. Check operator logs
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.name == "api-route")'
```

**Common causes**: DNSPolicy inactive (sourceRegion mismatch), Gateway not found, ClusterIdentity missing.

### DNS Records Not Created

```bash
# 1. Verify DNSEndpoints exist and have correct annotation
kubectl get dnsendpoints -n myapp
kubectl get dnsendpoint -n myapp <name> \
  -o jsonpath='{.metadata.annotations.external-dns\.alpha\.kubernetes\.io/controller}'

# 2. Check ExternalDNS logs
kubectl logs -n external-dns -l app=external-dns-weu

# 3. Verify DNS record in Azure
az network private-dns record-set cname show \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n api-ns-p-prod-myapp
```

**Common causes**: ExternalDNS not running, annotation mismatch, ownership conflict (different TXT owner ID), DNS provider permissions.

### DNSPolicy Inactive State

When ServiceRoute shows `phase: Pending, reason: DNSPolicyInactive`, the DNSPolicy is intentionally inactive on this cluster.

**In RegionBound mode**, this is expected on non-source clusters — only the `sourceRegion` cluster manages DNS. See [Architecture — Operational Modes](ARCHITECTURE.md#operational-modes) for details.

To debug:

```bash
# Verify ClusterIdentity region
kubectl get clusteridentity -o yaml | yq '.spec'

# Verify DNSPolicy mode and sourceRegion
kubectl get dnspolicy -n <namespace> -o yaml | yq '.spec'

# Check DNSPolicy active status
kubectl get dnspolicy -n <namespace> -o yaml | yq '.status'
```

### Increase Log Verbosity

```bash
kubectl edit deployment -n service-router-system service-router-operator
# Set: --zap-log-level=debug
```

### High Memory Usage

```bash
# Check current usage
kubectl top pod -n service-router-system

# Increase limit if needed
# resources.limits.memory: 512Mi
```

---

## Related Documentation

- [Architecture](ARCHITECTURE.md) — Controllers, CRDs, DNS flow
- [ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md) — ExternalDNS configuration
- [User Guide](USER-GUIDE.md) — Application team guide
- [Installation Guide](INSTALLATION.md) — Deployment procedures
