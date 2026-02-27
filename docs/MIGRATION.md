# Migration Guide: Helm Chart to Operator

This guide helps you migrate from the Service Router Helm chart to the Service Router Operator.

## Table of Contents

- [Overview](#overview)
- [Key Differences](#key-differences)
- [Migration Strategy](#migration-strategy)
- [Step-by-Step Migration](#step-by-step-migration)
- [Helm Values to CRD Mapping](#helm-values-to-crd-mapping)
- [Validation](#validation)
- [Rollback Plan](#rollback-plan)
- [Switching Between Active and RegionBound Modes](#switching-between-active-and-regionbound-modes)
- [Common Migration Issues](#common-migration-issues)

## Overview

### Why Migrate?

The Helm chart used templating to generate Istio Gateways and DNSEndpoints. This approach had limitations:

**Helm Chart Issues**:
- ❌ Static templates, no runtime adaptation
- ❌ Helm acts as orchestrator (reconciliation of reconciliation)
- ❌ Manual intervention needed for changes
- ❌ Limited multi-tenancy support
- ❌ Complex template logic

**Operator Benefits**:
- ✅ Continuous reconciliation (self-healing)
- ✅ React to changes automatically
- ✅ Clear separation of concerns (4 CRDs)
- ✅ Native multi-tenancy with RBAC
- ✅ Kubernetes-native resource management

### What Gets Migrated?

| Helm Chart Concept | Operator Resource |
|-------------------|-------------------|
| Global cluster/region config | ClusterIdentity (cluster-scoped) |
| Gateway definitions | Gateway CRDs (namespace-scoped) |
| App-level DNS config | DNSPolicy (namespace-scoped) |
| Service configurations | ServiceRoute (namespace-scoped) |

## Key Differences

### Resource Model

**Helm Chart** (monolithic):
```
One HelmRelease
  ├── Multiple gateways
  ├── Multiple apps
  └── Multiple services per app
```

**Operator** (granular):
```
ClusterIdentity (1 per cluster)
  ├── Gateway (N per cluster)
  ├── DNSPolicy (N per namespace)
  └── ServiceRoute (N per service)
```

### Reconciliation

**Helm Chart**:
- Deploy once, manual updates
- Helm upgrade needed for changes
- No automatic recovery

**Operator**:
- Continuous reconciliation
- Automatic updates on changes
- Self-healing

### Multi-Tenancy

**Helm Chart**:
- All configuration in one place
- Platform team manages everything
- Limited RBAC options

**Operator**:
- Clear ownership boundaries
- Application teams manage DNSPolicy + ServiceRoute
- Platform team manages ClusterIdentity + Gateway
- Namespace-level RBAC

### DNS Management

**Helm Chart**:
- Creates DNSEndpoints via Helm templates
- Must redeploy Helm release to change DNS

**Operator**:
- Creates DNSEndpoints automatically
- Updates DNS on ServiceRoute changes
- Watches related resources

## Migration Strategy

### Recommended Approach: Blue-Green Migration

Minimize risk by running Helm and Operator side-by-side temporarily.

```
┌─────────────────────────────────────────────────┐
│ Phase 1: Preparation (1-2 weeks)               │
│  - Install operator alongside Helm             │
│  - Migrate non-production services             │
│  - Validate behavior                           │
└─────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────┐
│ Phase 2: Gradual Migration (2-4 weeks)         │
│  - Migrate production services incrementally   │
│  - One namespace/app at a time                 │
│  - Monitor for issues                          │
└─────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────┐
│ Phase 3: Cleanup (1 week)                      │
│  - Remove Helm release                         │
│  - Clean up old resources                      │
│  - Update documentation                        │
└─────────────────────────────────────────────────┘
```

### Alternative: Big Bang Migration

Migrate all services at once (higher risk, but faster).

**Only recommended if**:
- Non-production environment
- Small number of services (< 10)
- Can tolerate potential downtime

## Step-by-Step Migration

### Prerequisites

1. ✅ Operator installed and running
2. ✅ ExternalDNS configured with CRD source
3. ✅ Istio installed
4. ✅ Access to current Helm values

### Phase 1: Install Operator

#### Step 1.1: Install CRDs

```bash
kubectl apply -f https://github.com/your-org/service-router-operator/releases/download/v1.0.0/crds.yaml
```

#### Step 1.2: Install Operator

```bash
helm install service-router-operator \
  oci://your-registry/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --version 1.0.0
```

#### Step 1.3: Verify Operator Running

```bash
kubectl get pods -n service-router-system
kubectl logs -n service-router-system deployment/service-router-operator
```

### Phase 2: Create ClusterIdentity

Extract cluster metadata from Helm values:

**From Helm `values.yaml`**:
```yaml
cluster: "aks01"
region: "weu"
environmentLetter: "p"
domain: "aks.vecp.vczc.nl"
```

**To ClusterIdentity**:
```bash
kubectl apply -f - <<EOF
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu
  cluster: aks01
  domain: aks.vecp.vczc.nl
  environmentLetter: p
EOF
```

**Verify**:
```bash
kubectl get clusteridentity cluster-identity -o yaml
```

### Phase 3: Create Gateways

Extract gateway configurations from Helm values:

**From Helm `values.yaml`**:
```yaml
gateways:
  - name: "default-gateway-ingress"
    controller: "aks-istio-ingressgateway-internal"
    credentialName: "cert-aks-ingress"
    targetPostfix: "internal"
```

**To Gateway CRD**:
```bash
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: default-gateway-ingress
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal
  credentialName: cert-aks-ingress
  targetPostfix: internal
EOF
```

**Verify**:
```bash
kubectl get gateways.routing.router.io -n istio-system
```

### Phase 4: Migrate Applications (Per Namespace)

For each application in Helm values, create DNSPolicy + ServiceRoutes.

#### Example Helm Configuration

```yaml
apps:
  - name: "nid-02"
    services:
      default-gateway-ingress:
        - "auth"
        - "pep"
        - "resource-server"
      internal-gateway-ingress:
        - "vecozo-claims-adapter"
    environment: dev
```

#### Step 4.1: Create DNSPolicy

Determine mode from Helm values:

**Helm `mode: active`** → **Operator `mode: Active`**
**Helm `mode: regionbound`** → **Operator `mode: RegionBound`**

```bash
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: nid-02-dns
  namespace: ns-d-dev-nid-02  # Your namespace
spec:
  mode: Active
  # Controllers from cluster-wide DNSConfiguration
EOF
```

**For RegionBound**:
```yaml
spec:
  mode: RegionBound
  sourceRegion: weu  # From Helm app.region
  externalDNSControllers:
    # ... same as above
```

#### Step 4.2: Create ServiceRoutes

One ServiceRoute per service:

```bash
# Auth service
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: auth-route
  namespace: ns-d-dev-nid-02
spec:
  serviceName: auth
  gatewayName: default-gateway-ingress
  gatewayNamespace: istio-system
  environment: dev
  application: nid-02
EOF

# PEP service
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: pep-route
  namespace: ns-d-dev-nid-02
spec:
  serviceName: pep
  gatewayName: default-gateway-ingress
  gatewayNamespace: istio-system
  environment: dev
  application: nid-02
EOF

# Resource server
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: resource-server-route
  namespace: ns-d-dev-nid-02
spec:
  serviceName: resource-server
  gatewayName: default-gateway-ingress
  gatewayNamespace: istio-system
  environment: dev
  application: nid-02
EOF

# Vecozo claims adapter (different gateway)
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: vecozo-claims-adapter-route
  namespace: ns-d-dev-nid-02
spec:
  serviceName: vecozo-claims-adapter
  gatewayName: internal-gateway-ingress
  gatewayNamespace: istio-system
  environment: dev
  application: nid-02
EOF
```

#### Step 4.3: Verify Migration for Namespace

```bash
# Check DNSPolicy active
kubectl get dnspolicy -n ns-d-dev-nid-02 -o yaml | yq '.status'

# Check ServiceRoutes ready
kubectl get serviceroutes -n ns-d-dev-nid-02

# Check DNSEndpoints created
kubectl get dnsendpoints -n ns-d-dev-nid-02

# Verify DNS still resolves
nslookup auth-ns-d-dev-nid-02.aks.vecd.vczc.nl
```

### Phase 5: Remove Helm Resources (Per Namespace)

Once operator resources are verified:

#### Step 5.1: Identify Helm-Managed Resources

```bash
# List DNSEndpoints created by Helm
kubectl get dnsendpoints -A -l app.kubernetes.io/managed-by=Helm

# List VirtualServices created by Helm  
kubectl get virtualservices -A -l app.kubernetes.io/managed-by=Helm
```

#### Step 5.2: Delete Helm-Created Resources

**Important**: Operator and Helm resources can coexist because they have different names.

Operator creates:
- DNSEndpoint: `{serviceroute-name}-{controller-name}`
- VirtualService: `{serviceroute-name}`

Helm creates:
- DNSEndpoint: `{controller-name}` (legacy naming)
- VirtualService: Various names

**To safely remove Helm resources**:

```bash
# Remove Helm resources for specific app
# (Do NOT remove the entire Helm release yet!)

# Option 1: Manually delete specific resources
kubectl delete dnsendpoint -n ns-d-dev-nid-02 external-dns-weu
kubectl delete dnsendpoint -n ns-d-dev-nid-02 external-dns-neu

# Option 2: Use Helm to remove specific app
# (requires updating Helm values to remove app, then upgrading)
```

### Phase 6: Complete Helm Removal

Once all apps are migrated:

```bash
# Final check: no Helm-managed DNSEndpoints or VirtualServices
kubectl get dnsendpoints -A -l app.kubernetes.io/managed-by=Helm
kubectl get virtualservices -A -l app.kubernetes.io/managed-by=Helm

# Uninstall Helm release
helm uninstall multi-region-service-router -n istio-system

# Clean up any remaining Helm-managed Istio Gateways
kubectl delete gateway -n istio-system -l app.kubernetes.io/managed-by=Helm
```

## Helm Values to CRD Mapping

### Global Configuration

| Helm Path | Operator Resource | Field |
|-----------|-------------------|-------|
| `cluster` | ClusterIdentity | `spec.cluster` |
| `region` | ClusterIdentity | `spec.region` |
| `environmentLetter` | ClusterIdentity | `spec.environmentLetter` |
| `domain` | ClusterIdentity | `spec.domain` |

**Example**:
```yaml
# Helm values.yaml
cluster: "aks01"
region: "weu"
environmentLetter: "p"
domain: "aks.vecp.vczc.nl"
```

**Becomes**:
```yaml
# ClusterIdentity
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  cluster: aks01
  region: weu
  environmentLetter: p
  domain: aks.vecp.vczc.nl
```

### Gateway Configuration

| Helm Path | Operator Resource | Field |
|-----------|-------------------|-------|
| `gateways[].name` | Gateway | `metadata.name` |
| `gateways[].controller` | Gateway | `spec.controller` |
| `gateways[].credentialName` | Gateway | `spec.credentialName` |
| `gateways[].targetPostfix` | Gateway | `spec.targetPostfix` |

**Example**:
```yaml
# Helm values.yaml
gateways:
  - name: "default-gateway"
    controller: "aks-istio-ingressgateway-internal"
    credentialName: "cert-aks-ingress"
    targetPostfix: "internal"
```

**Becomes**:
```yaml
# Gateway CRD
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: default-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal
  credentialName: cert-aks-ingress
  targetPostfix: internal
```

### ExternalDNS Configuration

| Helm Path | Operator Resource | Field |
|-----------|-------------------|-------|
| `externalDns[].controller` | DNSConfiguration | `spec.externalDNSControllers[].name` |
| `externalDns[].region` | DNSConfiguration | `spec.externalDNSControllers[].region` |

**Important**: ExternalDNS controllers are now defined in cluster-scoped DNSConfiguration, not in DNSPolicy.

**Example**:
```yaml
# Helm values.yaml
externalDns:
  - controller: external-dns-weu
    region: weu
  - controller: external-dns-neu
    region: neu
```

**Becomes**:
```yaml
# DNSConfiguration (cluster-scoped, created once)
apiVersion: cluster.router.io/v1alpha1
kind: DNSConfiguration
metadata:
  name: dns-config
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
---
# DNSPolicy (per namespace, references DNSConfiguration)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
  # Controllers come from DNSConfiguration
```

### Application Configuration

| Helm Path | Operator Resource | Field |
|-----------|-------------------|-------|
| `apps[].name` | ServiceRoute | `spec.application` |
| `apps[].environment` | ServiceRoute | `spec.environment` |
| `apps[].mode` | DNSPolicy | `spec.mode` (Active/RegionBound) |
| `apps[].region` | DNSPolicy | `spec.sourceRegion` (if RegionBound) |

### Service Configuration

| Helm Path | Operator Resource | Field |
|-----------|-------------------|-------|
| `apps[].services.{gateway}[]` | ServiceRoute | One per service |
| Service name in list | ServiceRoute | `spec.serviceName` |
| Gateway key | ServiceRoute | `spec.gatewayName` |

**Example**:
```yaml
# Helm values.yaml
apps:
  - name: "nid-02"
    services:
      default-gateway:
        - "auth"
        - "pep"
    environment: dev
```

**Becomes**:
```yaml
# ServiceRoute for auth
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: auth-route
  namespace: ns-d-dev-nid-02
spec:
  serviceName: auth
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: dev
  application: nid-02
---
# ServiceRoute for pep
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: pep-route
  namespace: ns-d-dev-nid-02
spec:
  serviceName: pep
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: dev
  application: nid-02
```

### Mode Mapping

| Helm Mode | Operator Mode | Additional Fields |
|-----------|---------------|-------------------|
| `active` | `Active` | None |
| `regionbound` | `RegionBound` | `sourceRegion` (required) |

**Example**:
```yaml
# Helm values.yaml
apps:
  - name: "dbadmin"
    mode: regionbound
    region: weu
    # ...
```

**Becomes**:
```yaml
# DNSPolicy
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: dbadmin-dns
  namespace: ns-p-prod-dbadmin
spec:
  mode: RegionBound
  sourceRegion: weu
  # Controllers from DNSConfiguration
```

## Validation

### DNS Records Match

Compare DNS records before and after migration:

```bash
# Before migration (Helm)
az network private-dns record-set cname show \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n auth-ns-d-dev-nid-02

# After migration (Operator)
az network private-dns record-set cname show \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n auth-ns-d-dev-nid-02

# Should be identical (same target)
```

### Traffic Routing Works

Test services after migration:

```bash
# Test DNS resolution
nslookup auth-ns-d-dev-nid-02.aks.vecd.vczc.nl

# Test HTTP request
curl https://auth-ns-d-dev-nid-02.aks.vecd.vczc.nl/health
```

### Resource Count Matches

Verify all services migrated:

```bash
# Count services in Helm values
cat helm-values.yaml | yq '.apps[].services[][] | length'

# Count ServiceRoutes created
kubectl get serviceroutes -A --no-headers | wc -l

# Should match
```

## Rollback Plan

If migration causes issues:

### Phase 1-4 Rollback (Before Helm Removal)

Both Helm and Operator running - safe to rollback:

```bash
# 1. Delete operator resources for affected namespace
kubectl delete serviceroutes -n ns-d-dev-nid-02 --all
kubectl delete dnspolicy -n ns-d-dev-nid-02 --all

# 2. Helm resources still exist - DNS continues working
# No action needed

# 3. Fix issues, try again
```

### Phase 5+ Rollback (After Helm Removal)

Helm resources deleted - need to restore:

```bash
# 1. Restore Helm release from backup values
helm install multi-region-service-router ./chart \
  -n istio-system \
  -f backup-values.yaml

# 2. Wait for DNS records to be recreated
sleep 60

# 3. Verify DNS working
nslookup auth-ns-d-dev-nid-02.aks.vecd.vczc.nl

# 4. Delete operator resources
kubectl delete serviceroutes -A --all
kubectl delete dnspolicies -A --all
kubectl delete gateways.routing.router.io -A --all
```

## Switching Between Active and RegionBound Modes

After migration, you may need to switch between Active and RegionBound modes for operational reasons.

### Active → RegionBound (Single-Cluster DNS Management)

**Use Case**: Consolidate DNS management to a single cluster for disaster recovery or cost optimization.

**Steps**:

1. **Choose the active cluster** (e.g., WEU cluster should manage all regions)

2. **Update DNSPolicy in WEU cluster**:
   ```yaml
   apiVersion: routing.router.io/v1alpha1
   kind: DNSPolicy
   metadata:
     name: default
     namespace: my-app
   spec:
     mode: RegionBound
     sourceRegion: weu  # Match WEU cluster's region
   ```

3. **Update DNSPolicy in other clusters (NEU, FRC)**:
   ```yaml
   apiVersion: routing.router.io/v1alpha1
   kind: DNSPolicy
   metadata:
     name: default
     namespace: my-app
   spec:
     mode: RegionBound
     sourceRegion: weu  # Does NOT match their region → inactive
   ```

4. **Verify cleanup in inactive clusters**:
   ```bash
   # On NEU and FRC clusters
   kubectl get dnsendpoints -n my-app
   # Should show no endpoints
   
   kubectl get serviceroute -n my-app
   # Status should be "Pending" with reason "DNSPolicyInactive"
   ```

5. **Verify WEU cluster manages all regions**:
   ```bash
   # On WEU cluster
   kubectl get dnsendpoints -n my-app
   # Should show endpoints for ALL regions (neu, weu, frc)
   
   kubectl get dnspolicy -n my-app -o yaml | yq '.status'
   # Should show: active: true, activeControllers: [external-dns-neu, external-dns-weu, external-dns-frc]
   ```

**Expected Downtime**: None (WEU cluster takes over DNS management seamlessly)

**What Happens**:
- WEU cluster creates DNSEndpoints for all regions
- NEU/FRC clusters delete their DNSEndpoints (cleanup)
- All DNS records now point to WEU cluster's gateway
- Clients in NEU/FRC regions route cross-region to WEU

### RegionBound → Active (Regional DNS Management)

**Use Case**: Return to regional isolation where each cluster manages its own region.

**Steps**:

1. **Update DNSPolicy in ALL clusters**:
   ```yaml
   apiVersion: routing.router.io/v1alpha1
   kind: DNSPolicy
   metadata:
     name: default
     namespace: my-app
   spec:
     mode: Active
     # Remove sourceRegion and sourceCluster
   ```

2. **Verify each cluster manages its own region**:
   ```bash
   # On NEU cluster
   kubectl get dnsendpoints -n my-app
   # Should show endpoints ONLY for "neu" region
   
   kubectl get dnspolicy -n my-app -o yaml | yq '.status'
   # Should show: active: true, activeControllers: [external-dns-neu]
   
   # On WEU cluster
   kubectl get dnsendpoints -n my-app
   # Should show endpoints ONLY for "weu" region
   
   kubectl get dnspolicy -n my-app -o yaml | yq '.status'
   # Should show: active: true, activeControllers: [external-dns-weu]
   ```

**Expected Downtime**: None (transition is seamless)

**What Happens**:
- Each cluster becomes active
- Each cluster creates DNSEndpoints for its own region only
- DNS records updated to point to regional gateways
- Clients route to their local regional cluster

### Rollback Procedure

If you need to rollback during mode switching:

1. **Restore previous DNSPolicy configuration from Git**:
   ```bash
   git checkout HEAD~1 -- k8s/dnspolicy.yaml
   kubectl apply -f k8s/dnspolicy.yaml
   ```

2. **Verify operator reconciles automatically**:
   ```bash
   # Watch ServiceRoute status
   kubectl get serviceroute -n my-app -w
   
   # Check DNSEndpoints
   kubectl get dnsendpoints -n my-app
   ```

3. **Verify DNS records stabilize within 2-3 minutes**:
   ```bash
   # Check DNS records in Azure
   az network private-dns record-set cname list \
     -g dns-rg \
     -z aks.vecp.vczc.nl \
     --query "[?name=='api-ns-p-prod-my-app'].{Name:name,CNAME:cnameRecord.cname}"
   ```

### Mode Switch Checklist

Before switching modes:

- [ ] Review traffic patterns and latency requirements
- [ ] Verify ExternalDNS is configured in all target regions
- [ ] Test in non-production environment first
- [ ] Have rollback plan ready
- [ ] Notify stakeholders of planned change
- [ ] Monitor DNS propagation after change
- [ ] Verify application health in all regions

After switching modes:

- [ ] Verify DNSPolicy status in all clusters
- [ ] Check DNSEndpoint creation matches expectations
- [ ] Validate DNS records in all DNS zones
- [ ] Test application access from different regions
- [ ] Monitor application metrics for anomalies
- [ ] Update documentation with new configuration

## Common Migration Issues

### Issue 1: DNS Records Duplicated

**Symptom**: Multiple CNAME records for same hostname

**Cause**: Both Helm and Operator creating records

**Solution**: This is expected during migration. Clean up after verifying operator works:

```bash
# Check which records exist
az network private-dns record-set cname list \
  -g dns-rg \
  -z aks.vecp.vczc.nl

# Delete Helm-created DNSEndpoints
kubectl delete dnsendpoint -n myapp external-dns-weu
```

### Issue 2: ServiceRoute NotReady - DNSPolicyNotFound

**Symptom**: ServiceRoute shows `DNSPolicyNotFound`

**Cause**: DNSPolicy not created before ServiceRoute

**Solution**: Create DNSPolicy first:

```bash
kubectl apply -f dnspolicy.yaml
kubectl apply -f serviceroute.yaml
```

### Issue 3: ServiceRoute NotReady - GatewayNotFound

**Symptom**: ServiceRoute shows `GatewayNotFound`

**Cause**: Gateway name mismatch or Gateway not created

**Solution**: 
```bash
# Check available Gateways
kubectl get gateways.routing.router.io -A

# Verify gateway name in ServiceRoute
kubectl get serviceroute -n myapp api-route -o yaml | yq '.spec.gatewayName'

# Fix or create Gateway
```

### Issue 4: Wrong DNS Target

**Symptom**: CNAME points to wrong cluster

**Cause**: DNSPolicy mode incorrect

**Solution**:
- **Want regional**: Use `mode: Active`
- **Want centralized**: Use `mode: RegionBound` with `sourceRegion`

```bash
# Check current mode
kubectl get dnspolicy -n myapp -o yaml | yq '.spec.mode'

# Update if needed
kubectl edit dnspolicy -n myapp myapp-dns
```

### Issue 5: Multiple TXT Records

**Symptom**: Several TXT ownership records

**Cause**: Normal during migration (Helm and Operator both create)

**Solution**: Not a problem, but clean up after Helm removal:

```bash
# Check TXT records
az network private-dns record-set txt list \
  -g dns-rg \
  -z aks.vecp.vczc.nl

# Manually delete old Helm TXT records if desired
az network private-dns record-set txt delete \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n old-txt-record
```

## Post-Migration Tasks

### Update Documentation

- [ ] Update runbooks to reference operator instead of Helm
- [ ] Update disaster recovery procedures
- [ ] Document new RBAC model
- [ ] Update onboarding guides for new teams

### Update CI/CD Pipelines

Replace Helm deployments with operator CRDs:

```yaml
# Old (Helm)
- helm upgrade multi-region-service-router ./chart

# New (Operator + CRDs)
- kubectl apply -f crds/serviceroute.yaml
```

### Training

Train teams on:
- Creating ServiceRoutes
- Understanding DNSPolicy modes
- Troubleshooting operator resources
- Using kubectl instead of Helm

### Monitoring

Set up alerts for:
- ServiceRoute NotReady conditions
- Operator pod failures
- DNS record inconsistencies

## Next Steps

After successful migration:

1. **Review [User Guide](USER-GUIDE.md)** for day-to-day usage
2. **Set up monitoring** as per [Operator Guide](OPERATOR-GUIDE.md)
3. **Document lessons learned** for future reference
4. **Consider contributing** improvements back to the project
