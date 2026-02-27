# Operator Guide

This guide is for platform engineers responsible for deploying, configuring, and operating the Service Router Operator.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Monitoring & Observability](#monitoring--observability)
- [Performance Considerations](#performance-considerations)
- [Upgrade Strategy](#upgrade-strategy)
- [Backup & Disaster Recovery](#backup--disaster-recovery)
- [Common Operational Tasks](#common-operational-tasks)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Components

Before deploying the Service Router Operator, ensure the following are installed:

| Component | Version | Purpose |
|-----------|---------|---------|
| **Kubernetes** | 1.24+ | Operator runtime |
| **Istio** | 1.18+ | Gateway resources (operator creates Gateways; users create VirtualServices) |
| **ExternalDNS** | 0.13+ | DNS record provisioning |
| **cert-manager** (optional) | 1.11+ | TLS certificate management |

### Cluster Requirements

- **RBAC enabled**: Operator needs cluster-wide permissions
- **CRD support**: Kubernetes API must support CustomResourceDefinitions
- **Network policy** (if used): Allow operator to access Kubernetes API server
- **DNS provider access**: ExternalDNS needs credentials for DNS provider

### ExternalDNS Setup

The operator requires ExternalDNS to be configured with:

```yaml
args:
  - --source=crd
  - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
  - --crd-source-kind=DNSEndpoint
  - --txt-owner-id=external-dns-{region}
  - --provider=azure-private-dns  # or your provider
```

**Note**: The operator's IngressDNS Controller creates DNSEndpoint CRDs for Gateway A records (LoadBalancer IPs). ExternalDNS only needs to watch CRDs, not Services directly.

See [ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md) for complete configuration.

### Istio Setup

Required Istio components:

1. **Istio Ingress Gateway** (LoadBalancer Service)
2. **TLS certificates** (as Kubernetes Secrets)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: aks-istio-ingressgateway-internal
  namespace: istio-system
  label: 
    istio: aks-istio-ingressgateway-internal
spec:
  type: LoadBalancer
```

## Installation

### Installation Methods

The Service Router Operator can be installed via:

1. **Helm Chart** (recommended for production)
2. **Kustomize** (for GitOps workflows)
3. **Direct YAML** (for development/testing)

See [Installation Guide](INSTALLATION.md) for detailed procedures.

### Quick Installation (Helm)

```bash
# Add Helm repository (if published)
helm repo add service-router https://your-repo/helm-charts
helm repo update

# Install operator
helm install service-router-operator service-router/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --set image.tag=v1.0.0
```

### Quick Installation (Kustomize)

```bash
# Install CRDs
kubectl apply -k config/crd

# Install operator
kubectl apply -k config/default
```

### Verification

Check that the operator is running:

```bash
# Check operator pod
kubectl get pods -n service-router-system
# Expected: service-router-operator-xxx Running

# Check CRDs installed
kubectl get crds | grep router.io
# Expected: clusteridentities.cluster.router.io
#           dnsconfigurations.cluster.router.io
#           gateways.routing.router.io
#           dnspolicies.routing.router.io
#           serviceroutes.routing.router.io

# Check operator logs
kubectl logs -n service-router-system deployment/service-router-operator
# Expected: No errors, "Starting workers"
```

## Configuration

### Operator Configuration

The operator can be configured via command-line flags or environment variables.

#### Available Flags

```bash
--metrics-bind-address=:8080          # Prometheus metrics endpoint
--health-probe-bind-address=:8081     # Health probe endpoint
--leader-elect=true                   # Enable leader election for HA
--zap-log-level=info                  # Log level (debug, info, warn, error)
--zap-time-encoding=iso8601           # Timestamp format
```

#### Environment Variables

```yaml
env:
  - name: POD_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
```

### Resource Limits

**Recommended Resource Configuration**:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

**Scaling Guidelines**:

| Cluster Size | ServiceRoutes | CPU Request | Memory Request |
|--------------|---------------|-------------|----------------|
| Small | < 50 | 100m | 64Mi |
| Medium | 50-200 | 200m | 128Mi |
| Large | 200-500 | 500m | 256Mi |
| X-Large | 500+ | 1000m | 512Mi |

### High Availability

For production deployments, run the operator with multiple replicas:

```yaml
spec:
  replicas: 2  # Or 3 for maximum availability
  template:
    spec:
      containers:
        - name: manager
          args:
            - --leader-elect=true  # REQUIRED for HA
```

**Leader Election**:

- Only one replica reconciles resources at a time
- Other replicas standby (hot backup)
- Automatic failover in < 30 seconds
- No split-brain scenarios

### RBAC Configuration

The operator requires cluster-wide permissions. Review the ClusterRole:

```bash
# View operator permissions
kubectl get clusterrole service-router-operator -o yaml
```

**Key Permissions**:

- **CRDs**: Full access to operator CRDs (ClusterIdentity, DNSConfiguration, Gateway, DNSPolicy, ServiceRoute)
- **Istio**: Create/update/delete Istio Gateway resources
- **ExternalDNS**: Create/update/delete DNSEndpoint CRDs
- **Core**: Watch Services (for Gateway LoadBalancer IPs), ConfigMaps, and Secrets (for TLS certs)

**Note**: The operator does NOT create Istio VirtualService resources. Users must create VirtualServices to route traffic to their services.

### Network Configuration

If using Network Policies, allow operator to:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: service-router-operator
  namespace: service-router-system
spec:
  podSelector:
    matchLabels:
      app: service-router-operator
  policyTypes:
    - Egress
  egress:
    # Allow Kubernetes API access
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443
    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              name: kube-system
      ports:
        - protocol: UDP
          port: 53
```

## Monitoring & Observability

### Metrics

The operator exposes Prometheus metrics on `:8080/metrics`.

#### Key Metrics

**Controller Metrics**:

```
# Reconciliation duration
controller_runtime_reconcile_time_seconds{controller="serviceroute"}

# Reconciliation rate
controller_runtime_reconcile_total{controller="serviceroute",result="success"}
controller_runtime_reconcile_total{controller="serviceroute",result="error"}

# Work queue depth
workqueue_depth{name="serviceroute"}

# Work queue latency
workqueue_queue_duration_seconds{name="serviceroute"}
```

**Resource Metrics**:

```
# Active resources
serviceroute_active_total
dnspolicy_active_total
gateway_active_total
```

#### Prometheus Configuration

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
      path: /metrics
```

#### Alerting Rules

**Recommended Alerts**:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: service-router-operator-alerts
spec:
  groups:
    - name: service-router-operator
      rules:
        # Operator not running
        - alert: ServiceRouterOperatorDown
          expr: up{job="service-router-operator"} == 0
          for: 5m
          severity: critical
          annotations:
            summary: Service Router Operator is down
        
        # High error rate
        - alert: ServiceRouterHighErrorRate
          expr: |
            rate(controller_runtime_reconcile_total{result="error"}[5m]) > 0.1
          for: 10m
          severity: warning
          annotations:
            summary: High reconciliation error rate
        
        # Slow reconciliation
        - alert: ServiceRouterSlowReconciliation
          expr: |
            histogram_quantile(0.99,
              rate(controller_runtime_reconcile_time_seconds_bucket[5m])
            ) > 30
          for: 15m
          severity: warning
          annotations:
            summary: Reconciliation taking too long
```

### Logging

The operator uses structured logging (JSON format).

#### Log Levels

```bash
# Debug (verbose)
--zap-log-level=debug

# Info (default)
--zap-log-level=info

# Warning (errors + warnings only)
--zap-log-level=warn

# Error (errors only)
--zap-log-level=error
```

#### Log Aggregation

**Recommended Setup**: Ship logs to centralized logging system (Loki, Elasticsearch, etc.)

```yaml
# Example: Fluentd DaemonSet configuration
<filter kubernetes.var.log.containers.service-router-operator-*.log>
  @type parser
  key_name log
  <parse>
    @type json
    time_key ts
    time_format %Y-%m-%dT%H:%M:%S.%NZ
  </parse>
</filter>
```

#### Useful Log Filters

```bash
# View all reconciliation errors
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.level == "error")'

# View ServiceRoute reconciliations
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.controller == "serviceroute")'

# View specific resource reconciliation
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.name == "api-route" and .namespace == "myapp")'
```

### Events

The operator creates Kubernetes Events for significant actions.

#### Viewing Events

```bash
# Events for specific ServiceRoute
kubectl describe serviceroute -n myapp api-route

# All operator-generated events
kubectl get events -A --field-selector involvedObject.apiVersion=routing.router.io/v1alpha1

# Recent events (last 1 hour)
kubectl get events -A --sort-by='.lastTimestamp' \
  | grep service-router-operator
```

#### Event Types

| Type | Reason | Description |
|------|--------|-------------|
| Normal | Created | Resource successfully created |
| Normal | Updated | Resource successfully updated |
| Normal | Deleted | Resource successfully deleted |
| Warning | FailedCreate | Failed to create dependent resource |
| Warning | FailedUpdate | Failed to update dependent resource |
| Warning | ValidationFailed | Resource spec validation failed |

### Status Conditions

Resources have status conditions that indicate their state.

#### Checking Status

```bash
# Check ServiceRoute status
kubectl get serviceroute -n myapp api-route -o yaml | yq '.status'

# Check all ServiceRoutes status
kubectl get serviceroutes -A -o custom-columns=\
NAME:.metadata.name,\
NAMESPACE:.metadata.namespace,\
READY:.status.conditions[?(@.type=="Ready")].status
```

#### Condition Types

**ServiceRoute**:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: ReconciliationSucceeded
      message: ServiceRoute is active
```

**DNSPolicy**:

```yaml
status:
  active: true
  activeControllers:
    - external-dns-weu
  conditions:
    - type: Ready
      status: "True"
      reason: PolicyActive
      message: DNSPolicy is active for this cluster
```

**Gateway**:

```yaml
status:
  phase: Active
  conditions:
    - type: Ready
      status: "True"
      reason: GatewayCreated
      message: Istio Gateway created successfully
```

## Performance Considerations

### Reconciliation Timing

**Default Behavior**:

- Reconciliation triggered by changes to watched resources
- Automatic requeue on error (exponential backoff)
- No periodic reconciliation by default

**Reconciliation Duration**:

- Typical: 100-500ms per ServiceRoute
- With DNS lookup: 200-1000ms
- With API rate limiting: 1-5 seconds

### Caching Behavior

**ClusterIdentity Cache**:

- In-memory cache populated on startup
- Updated when ClusterIdentity changes
- Reduces API calls significantly

**Client-Side Cache**:

- controller-runtime caches all watched resources
- Reduces load on Kubernetes API server
- Cache invalidation on resource changes

### Scale Limits

**Tested Scale**:

| Resource | Recommended Max | Tested Max |
|----------|----------------|------------|
| ServiceRoutes per namespace | 50 | 100 |
| Total ServiceRoutes | 500 | 1000 |
| DNSPolicies per namespace | 1 | 5 |
| Gateways per cluster | 10 | 20 |

**Performance Impact**:

- Each ServiceRoute creates 1-3 DNSEndpoints (depending on active controllers)
- Each DNSEndpoint creates 2 DNS records (CNAME + TXT)
- Large deployments (500+ ServiceRoutes) may require increased resources

### Optimization Tips

1. **Reduce reconciliation frequency**:
   - Group related changes (update multiple ServiceRoutes at once)
   - Use Flux Kustomization with `spec.interval` to batch updates

2. **Minimize cross-namespace references**:
   - Place Gateways in well-known namespace (`istio-system`)
   - Avoid frequently changing Gateway definitions

3. **Resource consolidation**:
   - Reuse Gateways across services
   - One DNSPolicy per namespace (not per service)

## Upgrade Strategy

### Upgrade Checklist

- [ ] Review release notes for breaking changes
- [ ] Backup CRDs (see [Backup & Disaster Recovery](#backup--disaster-recovery))
- [ ] Test upgrade in non-production environment
- [ ] Schedule maintenance window (if downtime expected)
- [ ] Notify application teams of upgrade
- [ ] Monitor operator logs after upgrade

### CRD Upgrades

**Important**: CRDs must be upgraded **before** the operator.

```bash
# 1. Upgrade CRDs
kubectl apply -f config/crd/bases/

# 2. Wait for CRDs to be established
kubectl wait --for condition=established --timeout=60s \
  crd/serviceroutes.routing.router.io

# 3. Upgrade operator
helm upgrade service-router-operator service-router/service-router-operator \
  --namespace service-router-system \
  --set image.tag=v1.1.0
```

### Rolling Upgrade

For zero-downtime upgrades:

```yaml
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0  # Always keep at least one replica
      maxSurge: 1        # Create new pod before terminating old
```

**Process**:

1. New pod starts, waits for leader election
2. Old pod finishes current reconciliations
3. Leadership transfers to new pod
4. Old pod terminates
5. Process repeats for other replicas

### Version Compatibility

| Operator Version | Kubernetes | Istio | ExternalDNS | CRD Version |
|------------------|------------|-------|-------------|-------------|
| v1.0.x | 1.24+ | 1.18+ | 0.13+ | v1alpha1 |
| v1.1.x | 1.24+ | 1.19+ | 0.14+ | v1alpha1 |

### Rollback Procedure

If upgrade fails:

```bash
# 1. Rollback operator deployment
helm rollback service-router-operator -n service-router-system

# 2. Verify operator is healthy
kubectl get pods -n service-router-system
kubectl logs -n service-router-system deployment/service-router-operator

# 3. Check resource status
kubectl get serviceroutes -A
```

**Note**: CRD rollback is **not supported**. If CRD changes cause issues, you must fix forward.

## Backup & Disaster Recovery

### What to Backup

1. **CRD Definitions**: `config/crd/bases/`
2. **Custom Resources**: All ClusterIdentity, DNSConfiguration, Gateway, DNSPolicy, ServiceRoute instances
3. **Operator Configuration**: Helm values, Kustomization files

### Backup Procedure

#### Manual Backup

```bash
# Backup all custom resources
kubectl get clusteridentities -o yaml > backup/clusteridentities.yaml
kubectl get dnsconfigurations -o yaml > backup/dnsconfigurations.yaml
kubectl get gateways -A -o yaml > backup/gateways.yaml
kubectl get dnspolicies -A -o yaml > backup/dnspolicies.yaml
kubectl get serviceroutes -A -o yaml > backup/serviceroutes.yaml

# Backup CRDs
kubectl get crd -o yaml \
  | grep -A 10000 "router.io" \
  > backup/crds.yaml
```

#### Automated Backup (Velero)

```yaml
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: service-router-backup
  namespace: velero
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  template:
    includedNamespaces:
      - "*"
    includedResources:
      - clusteridentities.cluster.router.io
      - dnsconfigurations.cluster.router.io
      - gateways.routing.router.io
      - dnspolicies.routing.router.io
      - serviceroutes.routing.router.io
    storageLocation: default
    ttl: 720h  # 30 days
```

### Restore Procedure

```bash
# 1. Ensure operator is running
kubectl get pods -n service-router-system

# 2. Restore CRDs (if needed)
kubectl apply -f backup/crds.yaml

# 3. Restore resources
kubectl apply -f backup/clusteridentities.yaml
kubectl apply -f backup/dnsconfigurations.yaml
kubectl apply -f backup/gateways.yaml
kubectl apply -f backup/dnspolicies.yaml
kubectl apply -f backup/serviceroutes.yaml

# 4. Verify reconciliation
kubectl get serviceroutes -A
kubectl get dnsendpoints -A
```

### Disaster Recovery Scenarios

#### Scenario 1: Operator Pod Deleted

**Impact**: None (automatically recreated by Deployment)

**Recovery**: Automatic (< 1 minute)

#### Scenario 2: CRDs Accidentally Deleted

**Impact**: All custom resources deleted (cascading delete)

**Recovery**:

```bash
# 1. Restore CRDs
kubectl apply -f backup/crds.yaml

# 2. Restore resources
kubectl apply -f backup/

# 3. Verify
kubectl get serviceroutes -A
```

**Prevention**: Use RBAC to restrict CRD deletion.

#### Scenario 3: Cluster Lost

**Impact**: All resources lost

**Recovery**:

1. Deploy new cluster
2. Install prerequisites (Istio, ExternalDNS)
3. Install operator
4. Restore resources from backup
5. Verify DNS records recreated

**Time to Recovery**: 15-30 minutes

## Common Operational Tasks

### Restarting the Operator

```bash
# Graceful restart
kubectl rollout restart deployment/service-router-operator -n service-router-system

# Force restart (delete pod)
kubectl delete pod -n service-router-system -l app=service-router-operator
```

### Debugging Reconciliation Issues

#### Check Resource Status

```bash
# ServiceRoute status
kubectl get serviceroute -n myapp api-route -o yaml | yq '.status'

# DNSPolicy status
kubectl get dnspolicy -n myapp -o yaml | yq '.status'

# Generated resources (DNSEndpoints and Istio Gateway)
kubectl get dnsendpoints -n myapp -l router.io/serviceroute=api-route
kubectl get gateway -n istio-system -o yaml  # Check aggregated hosts

# Note: VirtualServices are NOT created by operator (user responsibility)
```

#### Increase Log Level

```bash
# Edit deployment to set debug logging
kubectl edit deployment -n service-router-system service-router-operator

# Add/modify:
spec:
  template:
    spec:
      containers:
        - name: manager
          args:
            - --zap-log-level=debug
```

#### Trace Reconciliation

```bash
# Watch operator logs in real-time
kubectl logs -f -n service-router-system deployment/service-router-operator

# Filter for specific resource
kubectl logs -f -n service-router-system deployment/service-router-operator \
  | jq 'select(.name == "api-route")'
```

### Forcing Reconciliation

To force immediate reconciliation:

```bash
# Add annotation to trigger reconciliation
kubectl annotate serviceroute -n myapp api-route \
  reconcile.router.io/force="$(date +%s)"

# Or update the resource
kubectl patch serviceroute -n myapp api-route \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/environment", "value": "prod"}]'
```

### Cleaning Up Resources

#### Delete All ServiceRoutes in Namespace

```bash
kubectl delete serviceroutes -n myapp --all
```

**Note**: This automatically deletes owned DNSEndpoints (via OwnerReferences).

#### Clean Up Stale DNS Records

If DNSEndpoints are deleted but DNS records remain:

```bash
# Check for orphaned DNS records
az network private-dns record-set cname list \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  | jq '.[] | select(.fqdn | contains("myapp"))'

# Manually delete if needed
az network private-dns record-set cname delete \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n api-ns-p-prod-myapp
```

**Prevention**: Ensure ExternalDNS runs with `--policy=sync`.

### Namespace Cleanup

When deleting a namespace with operator resources:

```bash
# 1. Delete ServiceRoutes first (graceful)
kubectl delete serviceroutes -n myapp --all

# 2. Wait for DNSEndpoints to be removed
kubectl wait --for=delete dnsendpoint -n myapp --all --timeout=60s

# 3. Delete namespace
kubectl delete namespace myapp
```

## Troubleshooting

### Operator Not Starting

**Symptoms**:

- Pod in CrashLoopBackOff
- Errors in pod logs

**Common Causes**:

1. **CRDs not installed**:

```bash
kubectl get crd | grep router.io
# If empty, install CRDs
kubectl apply -f config/crd/bases/
```

1. **RBAC permissions missing**:

```bash
kubectl auth can-i get clusteridentities --as=system:serviceaccount:service-router-system:service-router-operator
# Should return "yes"
```

1. **Image pull failure**:

```bash
kubectl describe pod -n service-router-system -l app=service-router-operator
# Check Events section
```

### ServiceRoute Not Creating DNSEndpoints

**Symptoms**:

- ServiceRoute shows Ready=False
- No DNSEndpoints created

**Debugging Steps**:

```bash
# 1. Check ServiceRoute status
kubectl get serviceroute -n myapp api-route -o yaml | yq '.status'

# 2. Check conditions
kubectl get serviceroute -n myapp api-route \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'

# 3. Check DNSPolicy exists and is active
kubectl get dnspolicy -n myapp -o yaml | yq '.status'

# 4. Check Gateway exists
kubectl get gateway -n istio-system

# 5. Check ClusterIdentity exists
kubectl get clusteridentity

# 6. Check operator logs
kubectl logs -n service-router-system deployment/service-router-operator \
  | jq 'select(.name == "api-route")'
```

**Common Causes**:

1. **DNSPolicy not active**: `sourceRegion` doesn't match cluster
2. **Gateway not found**: Referenced Gateway doesn't exist
3. **ClusterIdentity missing**: Not configured on cluster
4. **Validation error**: Invalid ServiceRoute spec

### DNS Records Not Created

**Symptoms**:

- DNSEndpoints exist but no DNS records in provider

**Debugging Steps**:

```bash
# 1. Check DNSEndpoint exists
kubectl get dnsendpoints -n myapp

# 2. Check DNSEndpoint annotation
kubectl get dnsendpoint -n myapp api-route-external-dns-weu \
  -o jsonpath='{.metadata.annotations.external-dns\.alpha\.kubernetes\.io/controller}'

# 3. Check ExternalDNS logs
kubectl logs -n external-dns -l app=external-dns-weu

# 4. Check DNS records
az network private-dns record-set cname show \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n api-ns-p-prod-myapp
```

**Common Causes**:

1. **ExternalDNS not running**: Check ExternalDNS deployment
2. **Annotation mismatch**: DNSEndpoint annotation doesn't match ExternalDNS filter
3. **Ownership conflict**: Different owner ID has existing TXT record
4. **Permissions**: ExternalDNS cannot access DNS provider

### DNSPolicy Inactive State

**Symptoms**:

- ServiceRoute status is "Pending" with reason "DNSPolicyInactive"
- No DNSEndpoints created for the ServiceRoute
- Status message: "DNSPolicy is not active for this cluster (sourceRegion/sourceCluster mismatch). DNSEndpoints have been removed to prevent conflicts."

**What this means**:

The DNSPolicy in the ServiceRoute's namespace is configured as inactive for this cluster. This is **expected behavior** in RegionBound mode when the policy is intended for a different cluster/region.

**Common Causes**:

1. **RegionBound mode with non-matching sourceRegion**:
   - DNSPolicy has `spec.mode: RegionBound`
   - DNSPolicy has `spec.sourceRegion: weu`
   - But cluster is in region `neu`
   - **Expected**: This cluster should NOT manage DNS records

2. **RegionBound mode with non-matching sourceCluster**:
   - DNSPolicy has `spec.sourceCluster: aks01-weu`
   - But cluster name is `aks02-neu`
   - **Expected**: Only the specified cluster manages DNS

**Expected Behavior**:

When a DNSPolicy becomes inactive:
- ServiceRoute status shows "Pending"
- All DNSEndpoints for that ServiceRoute are deleted
- No DNS records are managed by this cluster
- Prevents race conditions with external-dns controllers in other regions

**Debugging Steps**:

```bash
# 1. Verify ClusterIdentity configuration
kubectl get clusteridentity -o yaml | yq '.spec'
# Check the region and cluster fields

# 2. Verify DNSPolicy configuration
kubectl get dnspolicy -n <namespace> -o yaml | yq '.spec'
# Check mode, sourceRegion, and sourceCluster

# 3. Check DNSPolicy status
kubectl get dnspolicy -n <namespace> -o yaml | yq '.status'
# Should show: active: false, activeControllers: []

# 4. Verify DNSEndpoints are cleaned up
kubectl get dnsendpoints -n <namespace>
# Should show NO endpoints for ServiceRoutes with inactive policy

# 5. Check ServiceRoute status
kubectl get serviceroute -n <namespace> <name> -o yaml | yq '.status'
# Should show phase: Pending, reason: DNSPolicyInactive
```

**How to Activate**:

To make the DNSPolicy active on this cluster, choose one of:

**Option 1: Change to Active mode (regional management)**
```yaml
kubectl patch dnspolicy -n <namespace> <name> --type=merge -p '
spec:
  mode: Active
  sourceRegion: null
  sourceCluster: null
'
```

**Option 2: Match sourceRegion to this cluster**
```bash
# First, get the cluster's region
REGION=$(kubectl get clusteridentity -o jsonpath='{.items[0].spec.region}')

# Then update the DNSPolicy
kubectl patch dnspolicy -n <namespace> <name> --type=merge -p "
spec:
  sourceRegion: $REGION
"
```

**Option 3: Match sourceCluster to this cluster**
```bash
# Get the cluster's name
CLUSTER=$(kubectl get clusteridentity -o jsonpath='{.items[0].spec.cluster}')

# Update the DNSPolicy
kubectl patch dnspolicy -n <namespace> <name> --type=merge -p "
spec:
  sourceCluster: $CLUSTER
"
```

**After updating the DNSPolicy**:
- ServiceRoute will automatically reconcile within seconds
- DNSEndpoints will be recreated
- ServiceRoute status will change to "Active"
- DNS records will be provisioned by external-dns

**When Inactive State is Correct**:

If you're running **multi-cluster with RegionBound mode**:
- One cluster (e.g., WEU) should have `active: true` and manage all DNS
- Other clusters (e.g., NEU, FRC) should have `active: false` and NOT manage DNS
- This is the **intended design** to prevent DNS conflicts
- Do NOT activate DNSPolicy in inactive clusters unless you're changing the architecture

See [ExternalDNS Integration - Troubleshooting](EXTERNALDNS-INTEGRATION.md#troubleshooting) for more details.

### High Memory Usage

**Symptoms**:

- Operator pod OOMKilled
- Increasing memory consumption

**Debugging**:

```bash
# Check current memory usage
kubectl top pod -n service-router-system

# Check resource limits
kubectl get pod -n service-router-system -l app=service-router-operator \
  -o jsonpath='{.items[0].spec.containers[0].resources}'
```

**Solutions**:

1. **Increase memory limits**:

```yaml
resources:
  limits:
    memory: 512Mi  # Increase from 256Mi
```

2. **Check for resource leaks**:

```bash
# Enable memory profiling
kubectl port-forward -n service-router-system \
  deployment/service-router-operator 6060:6060

# Access pprof
curl http://localhost:6060/debug/pprof/heap > heap.prof
```

3. **Reduce cache size** (if available in future versions)

### Leader Election Issues

**Symptoms**:

- Multiple replicas reconciling same resource
- Conflicting updates

**Debugging**:

```bash
# Check which pod is leader
kubectl get lease -n service-router-system

# Check logs for leader election messages
kubectl logs -n service-router-system deployment/service-router-operator \
  | grep "leader"
```

**Solutions**:

1. **Ensure leader election enabled**:

```yaml
args:
  - --leader-elect=true
```

2. **Check lease timeout** (if election stuck)
3. **Restart operator** to force re-election

## Support and Community

### Getting Help

1. **Documentation**: Start with [Architecture](ARCHITECTURE.md) and [User Guide](USER-GUIDE.md)
2. **GitHub Issues**: Report bugs or request features
3. **Logs**: Always include operator logs when reporting issues
4. **Status**: Include resource status when troubleshooting

### Useful Links

- [GitHub Repository](https://github.com/your-org/service-router-operator)
- [Architecture Documentation](ARCHITECTURE.md)
- [ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md)
- [User Guide](USER-GUIDE.md)
- [Development Guide](DEVELOPMENT.md)
