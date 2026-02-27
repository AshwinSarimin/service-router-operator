# ExternalDNS Integration

This document provides a comprehensive guide to how the Service Router Operator integrates with ExternalDNS for DNS record provisioning.

## Table of Contents

- [Overview](#overview)
- [How ExternalDNS Works](#how-externaldns-works)
- [Integration Architecture](#integration-architecture)
- [ExternalDNS Configuration](#externaldns-configuration)
- [DNSEndpoint Structure](#dnsendpoint-structure)
- [Owner ID and TXT Records](#owner-id-and-txt-records)
- [Cross-Cluster DNS Takeover](#cross-cluster-dns-takeover)
- [Gateway Hostname Resolution](#gateway-hostname-resolution)
- [Multi-Region DNS Management](#multi-region-dns-management)
- [Troubleshooting](#troubleshooting)

## Overview

The Service Router Operator **does not directly create DNS records**. Instead, it creates `DNSEndpoint` Custom Resources that the ExternalDNS controller watches and uses to provision actual DNS records in Azure Private DNS (or other DNS providers).

### Division of Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **Service Router Operator** | Creates DNSEndpoint CRDs with proper hostnames, targets, and metadata |
| **ExternalDNS** | Watches DNSEndpoint CRDs and creates/updates/deletes DNS records in DNS provider |

### Why This Design?

1. **Separation of Concerns**: Operator handles routing logic, ExternalDNS handles DNS provider integration
2. **Provider Agnostic**: Operator doesn't need DNS provider-specific code
3. **Proven Solution**: ExternalDNS is a mature, well-tested project
4. **Kubernetes-Native**: Uses standard CRD pattern for extensibility

## How ExternalDNS Works

ExternalDNS is a Kubernetes controller that synchronizes Kubernetes resources with DNS providers.

### Basic Flow

```
┌──────────────────┐
│   Kubernetes     │
│   Resources      │
│                  │
│  - Services      │
│  - Ingresses     │
│  - DNSEndpoints  │ ◄─── Service Router Operator creates these
└────────┬─────────┘
         │
         │ (watches)
         ▼
┌──────────────────┐
│  ExternalDNS     │
│  Controller      │
│                  │
│  - Reads sources │
│  - Filters       │
│  - Plans changes │
└────────┬─────────┘
         │
         │ (creates/updates/deletes)
         ▼
┌──────────────────┐
│   DNS Provider   │
│                  │
│  - Azure         │
│  - AWS Route53   │
│  - Google DNS    │
│  - etc.          │
└──────────────────┘
```

### Source Types

ExternalDNS can watch multiple source types:

| Source | What It Watches | Used By Operator? |
|--------|----------------|-------------------|
| `service` | Kubernetes Services with annotations | No |
| `ingress` | Ingress resources | No |
| `crd` | DNSEndpoint CRDs | **Yes (only)** |
| `istio-gateway` | Istio Gateway resources | No |

The Service Router Operator uses:

- **CRD source** for application DNS records (ServiceRoute → DNSEndpoint, Ingress LoadbalancerIP → DNSEndpoint)

## Integration Architecture

### Complete Integration Flow

```
┌────────────────────────────────────────────────────────────────────┐
│                   Service Router Operator                          │
│                                                                    │
│  ServiceRoute Controller:                                          │
│    1. Reads ServiceRoute, DNSPolicy, Gateway, ClusterIdentity      │
│    2. Generates DNSEndpoint CRDs                                   │
│    3. Sets controller annotation and owner ID                      │
│    4. Creates DNSEndpoints in namespace                            │
└─────────────────────────┬──────────────────────────────────────────┘
                          │
                          │ Creates DNSEndpoint CRDs
                          ▼
┌────────────────────────────────────────────────────────────────────┐
│                      DNSEndpoint CRDs                              │
│                                                                    │
│  apiVersion: externaldns.k8s.io/v1alpha1                           │
│  kind: DNSEndpoint                                                 │
│  metadata:                                                         │
│    annotations:                                                    │
│      external-dns.alpha.kubernetes.io/controller:                  │
│        external-dns-weu                                            │
│  spec:                                                             │
│    endpoints:                                                      │
│      - dnsName: api-ns-p-prod-myapp.example.com                    │
│        recordType: CNAME                                           │
│        targets:                                                    │
│          - aks01-weu-gateway.example.com                           │
└─────────────────────────┬──────────────────────────────────────────┘
                          │
                          │ Watches with label-filter
                          ▼
┌────────────────────────────────────────────────────────────────────┐
│                  ExternalDNS Controller                            │
│                  (external-dns-weu)                                │
│                                                                    │
│  Configuration:                                                    │
│    --source=crd                                                    │
│    --crd-source-kind=DNSEndpoint                                   │
│    --txt-owner-id=external-dns-weu                                 │
│    --label-filter=router.io/region=weu                             │
│    --provider=azure-private-dns                                    │
│    --azure-resource-group=dns-rg                                   │
│    --azure-subscription-id=...                                     │
└─────────────────────────┬──────────────────────────────────────────┘
                          │
                          │ Creates/Updates DNS records
                          ▼
┌────────────────────────────────────────────────────────────────────┐
│               Azure Private DNS Zone                               │
│               (aks.vecp.vczc.nl)                                   │
│                                                                    │
│  CNAME Record:                                                     │
│    api-ns-p-prod-myapp.aks.vecp.vczc.nl                            │
│    → aks01-weu-gateway.aks.vecp.vczc.nl                            │
│                                                                    │
│  TXT Record (ownership):                                           │
│    weu-p-aks01-api-ns-p-prod-myapp.aks.vecp.vczc.nl              │
│    → "external-dns-weu"                                            │
└────────────────────────────────────────────────────────────────────┘
```

## ExternalDNS Configuration

### Required Configuration

For the Service Router Operator to work with a specific region correctly, ExternalDNS must be deployed with specific configuration.

#### Minimum Configuration

This is a minimum configuration when deployed as a single instance for one region, in this case West Europe (weu):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns-weu
  namespace: external-dns
spec:
  template:
    spec:
      containers:
        - name: external-dns
          image: registry.k8s.io/external-dns/external-dns:v0.20.0
          args:
            # Source Configuration
            - --source=crd                                         # REQUIRED: Watch CRDs
            - --crd-source-apiversion=externaldns.k8s.io/v1alpha1  # REQUIRED
            - --crd-source-kind=DNSEndpoint                        # REQUIRED
            - "--managed-record-types=CNAME"
            - "--managed-record-types=A"
            - "--managed-record-types=TXT"

            # Filtering
            - --label-filter=router.io/region=weu
            
            # Ownership
            - --txt-owner-id=external-dns-weu                      # REQUIRED: Must match region
            - --txt-prefix=weu-p-aks01-                            # RECOMMENDED: Unique per cluster
            
            # Provider Configuration
            - --provider=azure-private-dns
            - --azure-resource-group=dns-rg
            - --azure-subscription-id=12345678-1234-1234-1234-123456789012
            
            # Domain Filtering
            - --domain-filter=aks.vecp.vczc.nl
            
            # Policy
            - --policy=upsert-only                                 # RECOMMENDED: To prevent accidental deletions of DNS records
            
            # Reconciliation
            - --interval=1m
```

#### Configuration Explained

##### Source Configuration

```yaml
- --source=crd                    # Watch DNSEndpoint CRDs (PRIMARY)
- --crd-source-apiversion=externaldns.k8s.io/v1alpha1
- --crd-source-kind=DNSEndpoint
- "--managed-record-types=CNAME"  # Only manage these record types
- "--managed-record-types=A"
- "--managed-record-types=TXT"
```

**Label Filter**:

- Only process DNSEndpoints with matching label
- Enables multiple ExternalDNS controllers in same cluster (filter between `weu` and `neu` for example)

```yaml
- --label-filter=router.io/region=weu
```

##### Ownership

```yaml
- --txt-owner-id=external-dns-weu
- --txt-prefix=weu-p-aks01-
```

**txt-owner-id**:

- **CRITICAL**: Must follow pattern `external-dns-{region}`
- Used for cross-cluster takeover within same region
- Must match the owner ID in DNSEndpoint TXT records

**txt-prefix**:

- Makes TXT records unique per cluster
- Format: `{region}-{env}-{cluster}-`
- Prevents TXT record collisions

##### Provider Configuration

```yaml
- --provider=azure-private-dns
- --azure-resource-group=dns-rg
- --azure-subscription-id=12345678-1234-1234-1234-123456789012
```

**Azure-Specific**:

- Requires Azure credentials (Workload Identity or MSI)
- Resource group contains the Private DNS Zone
- Subscription ID for authentication

##### Policy

```yaml
- --policy=upsert-only  # Recommended Default
# OR
- --policy=sync
```

**upsert-only** (Recommended):

- Only creates and updates records
- Never deletes records
- Old records remain after ServiceRoute deletion, this is safer for production as it prevents accidental deletions in DNS zones.

**sync**:

- Creates, updates, **and deletes** DNS records
- Cleans up stale records when DNSEndpoints are removed
- Prevents DNS record accumulation
- Useful for testing, but not recommended for production, as it will delete DNS records from the DNS Zone if DNSEndpoints are removed unwantedly

### Region-Specific Deployment

Each region should have its own ExternalDNS controller.

#### Example: West Europe

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns-weu
  namespace: external-dns
  labels:
    app: external-dns-weu
spec:
  selector:
    matchLabels:
      app: external-dns-weu
  template:
    metadata:
      labels:
        app: external-dns-weu
    spec:
      serviceAccountName: external-dns-weu
      containers:
        - name: external-dns
          image: registry.k8s.io/external-dns/external-dns:v0.14.0
          args:
            - --source=crd
            - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
            - --crd-source-kind=DNSEndpoint
            - --txt-owner-id=external-dns-weu
            - --txt-prefix=weu-p-aks01-
            - --provider=azure-private-dns
            - --azure-resource-group=weu-dns-rg
            - --azure-subscription-id=...
            - --domain-filter=aks.vecp.vczc.nl
            - --label-filter=router.io/region=weu
            - --policy=upsert-only
```

#### Example: North Europe

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns-neu
  namespace: external-dns
  labels:
    app: external-dns-neu
spec:
  selector:
    matchLabels:
      app: external-dns-neu
  template:
    metadata:
      labels:
        app: external-dns-neu
    spec:
      serviceAccountName: external-dns-neu
      containers:
        - name: external-dns
          image: registry.k8s.io/external-dns/external-dns:v0.14.0
          args:
            - --source=crd
            - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
            - --crd-source-kind=DNSEndpoint
            - --txt-owner-id=external-dns-neu     # Different region
            - --txt-prefix=neu-p-aks02-           # Different cluster
            - --provider=azure-private-dns
            - --azure-resource-group=neu-dns-rg   # Different resource group
            - --azure-subscription-id=...
            - --domain-filter=aks.vecp.vczc.nl
            - --label-filter=router.io/region=neu # Different resource group
            - --policy=upsert-only
```

**Key Differences**:

- Different `--txt-owner-id` (region-based)
- Different `--txt-prefix` (cluster-based)
- Different `--azure-resource-group` (DNS zone location)
- Different `--label-filter` (DNSEndpoints controller selection)

### Authentication

ExternalDNS needs permissions to manage DNS zones.

#### Azure Workload Identity

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-dns-weu
  namespace: external-dns
  annotations:
    azure.workload.identity/client-id: "12345678-1234-1234-1234-123456789012"
    azure.workload.identity/tenant-id: "87654321-4321-4321-4321-210987654321"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns-weu
spec:
  template:
    metadata:
      labels:
        azure.workload.identity/use: "true"
    spec:
      serviceAccountName: external-dns-weu
      # ... container spec
```

**Required Azure RBAC**:

```bash
# DNS Zone Contributor on Private DNS Zone
az role assignment create \
  --assignee <managed-identity-client-id> \
  --role "Private DNS Zone Contributor" \
  --scope /subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Network/privateDnsZones/<zone>
```

## DNSEndpoint Structure

### Complete Example

This is what the operator creates:

```yaml
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: api-route-external-dns-weu
  namespace: myapp
  labels:
    app.kubernetes.io/managed-by: service-router-operator
    router.io/controller: external-dns-weu
    router.io/region: weu
    router.io/serviceroute: api-route
    router.io/source-namespace: myapp
  annotations:
    external-dns.alpha.kubernetes.io/controller: external-dns-weu
  ownerReferences:
    - apiVersion: routing.router.io/v1alpha1
      kind: ServiceRoute
      name: api-route
      uid: 12345678-1234-1234-1234-123456789012
      controller: true
      blockOwnerDeletion: true
spec:
  endpoints:
    # CNAME: Application hostname → Gateway hostname
    - dnsName: api-ns-p-prod-myapp.aks.vecp.vczc.nl
      recordType: CNAME
      targets:
        - aks01-weu-internal.aks.vecp.vczc.nl
      recordTTL: 300
```

### Field Explanations

#### Metadata

**Name**: `{serviceroute-name}-{controller-name}`

- Ensures uniqueness when multiple controllers manage same ServiceRoute
- Example: `api-route-external-dns-weu`

**Namespace**: Same as ServiceRoute

- Enables automatic cleanup via OwnerReference
- ExternalDNS must watch all relevant namespaces

**Labels**:

```yaml
app.kubernetes.io/managed-by: service-router-operator  # Identify operator-created resources
router.io/controller: external-dns-weu                 # Logical controller name
router.io/region: weu                                  # Geographic region
router.io/serviceroute: api-route                      # Link back to ServiceRoute
router.io/source-namespace: myapp                      # Original namespace
```

- `router.io/region: weu` must match the externalDNS `--label-filter`
  - Determines which ExternalDNS controller processes this resource

**Annotations**:

```yaml
external-dns.alpha.kubernetes.io/controller: external-dns-weu
```

**OwnerReferences**:

```yaml
ownerReferences:
  - apiVersion: routing.router.io/v1alpha1
    kind: ServiceRoute
    name: api-route
```

- Links DNSEndpoint to ServiceRoute
- Enables automatic deletion when ServiceRoute is deleted
- Only works if DNSEndpoint is in same namespace

#### Spec

**endpoints[0]** - CNAME Record:

```yaml
- dnsName: api-ns-p-prod-myapp.aks.vecp.vczc.nl      # What clients look up
  recordType: CNAME
  targets:
    - aks01-weu-internal.aks.vecp.vczc.nl            # Where it points
  recordTTL: 300                                      # Cache duration (optional)
```

### Multiple DNSEndpoints per ServiceRoute

When a ServiceRoute is in a namespace with multiple active controllers (RegionBound mode), the operator creates **one DNSEndpoint per controller**.

#### Example: RegionBound Mode

**DNSPolicy**:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
status:
  active: true
  activeControllers:
    - external-dns-weu
    - external-dns-neu
```

**ServiceRoute**:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: myapp
spec:
  serviceName: api
  gatewayName: default-gateway
  environment: prod
  application: myapp
```

**Generated DNSEndpoints**:

1. **api-route-external-dns-weu**:

```yaml
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: api-route-external-dns-weu
  namespace: myapp
  annotations:
    external-dns.alpha.kubernetes.io/controller: external-dns-weu
spec:
  endpoints:
    - dnsName: api-ns-p-prod-myapp.aks.vecp.vczc.nl
      recordType: CNAME
      targets:
        - aks01-weu-internal.aks.vecp.vczc.nl
```

2. **api-route-external-dns-neu**:

```yaml
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: api-route-external-dns-neu
  namespace: myapp
  annotations:
    external-dns.alpha.kubernetes.io/controller: external-dns-neu  # Different!
spec:
  endpoints:
    - dnsName: api-ns-p-prod-myapp.aks.vecp.vczc.nl  # Same hostname
      recordType: CNAME
      targets:
        - aks01-weu-internal.aks.vecp.vczc.nl        # Same target (WEU gateway)
```

**Result**:

- WEU ExternalDNS creates record in WEU DNS zone
- NEU ExternalDNS creates record in NEU DNS zone
- Both records point to WEU cluster
- Clients in both regions route to WEU cluster

## Owner ID and TXT Records

### How Ownership Works

ExternalDNS uses TXT records to track ownership of DNS records. This enables safe multi-cluster operation.

#### TXT Record Format

When using `--txt-owner-id` with `--txt-prefix`, ExternalDNS creates TXT records with:

```
Name: {txt-prefix}{dnsName}
Type: TXT
Value: "{txt-owner-id}"
```

#### Example

**Configuration**:

```yaml
--txt-owner-id=external-dns-weu
--txt-prefix=weu-p-aks01-
```

**DNS Record**:

```
Name: api-ns-p-prod-myapp.aks.vecp.vczc.nl
Type: CNAME
Target: aks01-weu-internal.aks.vecp.vczc.nl
```

**TXT Record** (created by ExternalDNS):

```
Name: weu-p-aks01-api-ns-p-prod-myapp.aks.vecp.vczc.nl
Type: TXT
Value: "external-dns-weu"
```

### Ownership Rules

1. **Create**: If no TXT record exists, ExternalDNS creates both the DNS record and TXT record
2. **Update**: If TXT record exists with matching `owner`, ExternalDNS can update the DNS record
3. **Delete**: If TXT record exists with matching `owner`, ExternalDNS can delete the DNS record
4. **Ignore**: If TXT record exists with **different** `owner`, ExternalDNS ignores the DNS record

### Why This Matters

**Safety**: Prevents different ExternalDNS instances from interfering with each other's records.

**Scenario**:

- WEU cluster has ExternalDNS with `--txt-owner-id=external-dns-weu`
- NEU cluster has ExternalDNS with `--txt-owner-id=external-dns-neu`
- If WEU creates a record, NEU **cannot** modify it (different owner)
- If WEU fails, NEU **cannot** take over (different owner)

## Cross-Cluster DNS Takeover

The Service Router Operator enables **cross-cluster DNS takeover** within the same region by using **shared owner IDs**.

### How Takeover Works

**Key Insight**: If two ExternalDNS instances in the same region use the **same** `--txt-owner-id`, they can take ownership of each other's records.

#### Configuration

**Cluster 1** (aks01-weu):

```yaml
--txt-owner-id=external-dns-weu
--txt-prefix=weu-p-aks01-
```

**Cluster 2** (aks02-weu):

```yaml
--txt-owner-id=external-dns-weu     # Same owner ID!
--txt-prefix=weu-p-aks02-           # Different prefix
```

#### Takeover Scenario

**Initial State** (aks01 is active):

| Cluster | Owner ID | DNS Record | TXT Record Owner |
|---------|----------|------------|------------------|
| aks01 | external-dns-weu | api.example.com → aks01-weu-gateway | external-dns-weu |
| aks02 | external-dns-weu | (none) | (none) |

**After Failover** (ServiceRoute moved to aks02):

1. User updates ServiceRoute to use aks02 gateway (or deploys ServiceRoute to aks02)
2. Operator creates DNSEndpoint in aks02
3. aks02 ExternalDNS sees DNSEndpoint
4. aks02 ExternalDNS checks TXT record: owner is `external-dns-weu` (matches!)
5. aks02 ExternalDNS **takes ownership** and updates DNS record

| Cluster | Owner ID | DNS Record | TXT Record Owner |
|---------|----------|------------|------------------|
| aks01 | external-dns-weu | (stale) | external-dns-weu |
| aks02 | external-dns-weu | api.example.com → aks02-weu-gateway | external-dns-weu |

**Result**: DNS record now points to aks02, even though it was created by aks01.

### Important Considerations

#### Same Region Only

Cross-cluster takeover **only works within the same region**.

**Why?**

- Owner ID is `external-dns-{region}`
- Different regions = different owner IDs
- Different owner IDs = cannot take ownership

**Example**:

- WEU cluster: `--txt-owner-id=external-dns-weu`
- NEU cluster: `--txt-owner-id=external-dns-neu`
- NEU **cannot** take over WEU's records (different owners)

#### Manual Failover

The operator **does not automatically detect failures**. Takeover requires explicit action:

1. Update ServiceRoute to reference different Gateway
2. Delete ServiceRoute in old cluster, create in new cluster
3. Update DNSPolicy `sourceRegion` to activate in different cluster

**Why No Auto-Failover?**

- Prevents accidental split-brain scenarios
- Requires conscious decision by operator/platform team
- Aligns with Kubernetes' declarative model

#### TXT Record Accumulation

Old TXT records remain after takeover (unless `--policy=sync`).

**Example**:

```
weu-p-aks01-api.example.com → "external-dns-weu"
weu-p-aks02-api.example.com → "external-dns-weu"
```

Both TXT records have the same owner value, but different prefixes in the record name.

**Solution**:

- Use `--policy=sync` to clean up stale records
- Manually delete old TXT records
- They don't affect functionality, just clutter
- But beware, this also means that a DNSEndpoint that deleted by accident will also delete the DNS record, which is not desired in production environments.

## Gateway Hostname Resolution

The operator creates CNAME records pointing to gateway hostnames (e.g., `aks01-weu-internal.example.com`). How do these resolve to IPs?

### Two-Step DNS Resolution

**Step 1**: Application hostname → Gateway hostname (CNAME)

- Created by: Service Router Operator ServiceRoute Controller (via DNSEndpoint CRD)
- Record: `api-ns-p-prod-myapp.example.com` → `aks01-weu-internal.example.com`

**Step 2**: Gateway hostname → Load Balancer IP (A record)

- Created by: Service Router Operator IngressDNS Controller (via DNSEndpoint CRD)
- Record: `aks01-weu-internal.example.com` → `10.123.45.67`

### How IngressDNS Controller Creates A Records

The Service Router Operator's **IngressDNS Controller** manages DNS A records for Gateway infrastructure.

#### Process

1. **Watches Gateway CRDs** and LoadBalancer Services
2. **Constructs gateway hostname** from ClusterIdentity + Gateway targetPostfix
3. **Gets LoadBalancer IP** from Service status
4. **Creates DNSEndpoint CRD** with A record (one per ExternalDNS controller)

#### Example

**Gateway CRD**:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: default-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal
  targetPostfix: internal
```

**LoadBalancer Service** (Istio Gateway):

```yaml
apiVersion: v1
kind: Service
metadata:
  name: aks-istio-ingressgateway-internal
  namespace: istio-system
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
    service.beta.kubernetes.io/azure-load-balancer-internal-subnet: "aks-lb-subnet"
spec:
  type: LoadBalancer
  selector:
    istio: aks-istio-ingressgateway-internal
  ports:
    - name: https
      port: 443
      protocol: TCP
      targetPort: 8443
status:
  loadBalancer:
    ingress:
      - ip: 10.123.45.67
```

**DNSEndpoint Created by IngressDNS Controller**:

```yaml
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: gateway-controller-aks-istio-ingressgateway-internal-internal-external-dns-weu
  namespace: istio-system
  labels:
    router.io/istio-controller: aks-istio-ingressgateway-internal
    router.io/target-postfix: internal
    router.io/region: weu
    router.io/resource-type: gateway-service
  annotations:
    external-dns.alpha.kubernetes.io/controller: external-dns-weu
spec:
  endpoints:
    - dnsName: aks01-weu-internal.aks.vecp.vczc.nl
      recordType: A
      targets:
        - 10.123.45.67
```

**Note**: The operator does NOT use Service annotations for DNS. All DNS records (both CNAME and A records) are managed through DNSEndpoint CRDs.

### Complete DNS Chain

```
Client Query: api-ns-p-prod-myapp.aks.vecp.vczc.nl
    ↓
DNS Server: CNAME lookup
    ↓
CNAME Record (from ServiceRoute Controller → DNSEndpoint CRD):
    api-ns-p-prod-myapp.aks.vecp.vczc.nl
    → aks01-weu-internal.aks.vecp.vczc.nl
    ↓
DNS Server: A record lookup
    ↓
A Record (from IngressDNS Controller → DNSEndpoint CRD):
    aks01-weu-internal.aks.vecp.vczc.nl
    → 10.123.45.67
    ↓
Client receives: 10.123.45.67
    ↓
Client connects to Load Balancer
```

### Why CNAME + A Record?

**Benefits**:

1. **Flexibility**: Change gateway IP without updating all service records
2. **Reusability**: Multiple services point to same gateway hostname
3. **Clarity**: Clear separation between service DNS and infrastructure DNS
4. **Maintenance**: Update gateway once, affects all services

## Multi-Region DNS Management

### Active Mode - Regional DNS

Each cluster manages DNS **only for its own region**.

#### Configuration

**WEU Cluster**:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
  # Controllers from DNSConfiguration
status:
  active: true
  activeControllers:
    - external-dns-weu  # Only WEU (from DNSConfiguration)!
```

**NEU Cluster**:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
status:
  active: true
  activeControllers:
    - external-dns-neu  # Only NEU!
```

#### DNS Records Created

**WEU DNS Zone** (managed by WEU cluster):

```
api-ns-p-prod-myapp.aks.vecp.vczc.nl → aks01-weu-internal.aks.vecp.vczc.nl
```

**NEU DNS Zone** (managed by NEU cluster):

```
api-ns-p-prod-myapp.aks.vecp.vczc.nl → aks02-neu-internal.aks.vecp.vczc.nl
```

#### Traffic Flow

- **WEU Clients**: Query WEU DNS zone → Route to aks01 (WEU cluster)
- **NEU Clients**: Query NEU DNS zone → Route to aks02 (NEU cluster)

### RegionBound Mode - Cross-Region DNS

One cluster manages DNS for **multiple regions**.

#### Configuration

**WEU Cluster** (active):

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Only activate in WEU
  # All controllers from DNSConfiguration will be used
status:
  active: true           # Active because region matches
  activeControllers:     # Populated from DNSConfiguration
    - external-dns-weu
    - external-dns-neu
```

**NEU Cluster** (inactive):

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Doesn't match NEU
  # Controllers from DNSConfiguration
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
status:
  active: false          # Inactive because region doesn't match
  activeControllers: []  # No controllers active
```

#### DNS Records Created

**WEU DNS Zone** (managed by WEU cluster):

```
api-ns-p-prod-myapp.aks.vecp.vczc.nl → aks01-weu-internal.aks.vecp.vczc.nl
```

**NEU DNS Zone** (managed by WEU cluster):

```
api-ns-p-prod-myapp.aks.vecp.vczc.nl → aks01-weu-internal.aks.vecp.vczc.nl
                                         ↑
                                  Same target (WEU)!
```

#### Traffic Flow

- **WEU Clients**: Query WEU DNS zone → Route to aks01 (WEU cluster)
- **NEU Clients**: Query NEU DNS zone → Route to aks01 (WEU cluster, **cross-region**)

### Cross-Region Controller Configuration

**Scenario**: DNS zone in FRC region, but no cluster there.

#### Configuration

First, ensure **DNSConfiguration** includes the FRC controller:

```yaml
apiVersion: cluster.router.io/v1alpha1
kind: DNSConfiguration
metadata:
  name: dns-config
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-frc
      region: frc
```

Then create **DNSPolicy** in RegionBound mode:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu
  # All controllers from DNSConfiguration (including frc) will be used
```

#### ExternalDNS Deployment

Deploy `external-dns-frc` in WEU cluster:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns-frc
  namespace: external-dns
spec:
  template:
    spec:
      containers:
        - name: external-dns
          args:
            - --txt-owner-id=external-dns-frc
            - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-frc
            - --azure-resource-group=frc-dns-rg  # FRC DNS zone!
            # ... other args
```

#### Result

- WEU cluster creates DNSEndpoint with annotation `external-dns-frc`
- `external-dns-frc` controller (in WEU) processes it
- DNS record created in FRC DNS zone
- FRC clients can reach service (via WEU cluster)

## Troubleshooting

### Verifying ExternalDNS Configuration

#### Check ExternalDNS Logs

```bash
kubectl logs -n external-dns -l app=external-dns-weu
```

**Look For**:

- `All records are already up to date` (good)
- `Created DNS record` (creating new records)
- `Updated DNS record` (updating existing records)
- `Deleted DNS record` (cleaning up)
- `Ownership error` (cannot manage record)

#### Verify DNSEndpoints Are Processed

```bash
# List all DNSEndpoints
kubectl get dnsendpoints -A

# Describe specific DNSEndpoint
kubectl describe dnsendpoint -n myapp api-route-external-dns-weu

# Check annotation
kubectl get dnsendpoint -n myapp api-route-external-dns-weu \
  -o jsonpath='{.metadata.annotations.external-dns\.alpha\.kubernetes\.io/controller}'
```

#### Check Azure DNS Records

```bash
# List DNS records
az network private-dns record-set cname list \
  -g dns-rg \
  -z aks.vecp.vczc.nl

# Get specific record
az network private-dns record-set cname show \
  -g dns-rg \
  -z aks.vecp.vczc.nl \
  -n api-ns-p-prod-myapp
```

### Common Issues

#### DNSEndpoints Not Processed

**Symptom**: DNSEndpoints exist, but no DNS records created.

**Possible Causes**:

1. **Annotation Mismatch**:
   - DNSEndpoint annotation: `external-dns-weu`
   - ExternalDNS filter: `external-dns-neu`
   - Fix: Update DNSPolicy or ExternalDNS config

2. **ExternalDNS Not Watching CRDs**:
   - Missing `--source=crd`
   - Fix: Add to ExternalDNS args

3. **Permissions Issue**:
   - ExternalDNS cannot access Azure DNS
   - Check: Azure RBAC, Workload Identity
   - Fix: Grant "Private DNS Zone Contributor"

4. **Wrong Namespace**:
   - ExternalDNS watching specific namespaces only
   - Fix: Configure cluster-wide watch or add namespace

#### DNS Records Not Updating

**Symptom**: DNSEndpoint updated, but DNS record unchanged.

**Possible Causes**:

1. **Ownership Conflict**:
   - TXT record has different owner ID
   - Check: `az network private-dns record-set txt show ...`
   - Fix: Delete old TXT record or change owner ID

2. **Reconciliation Not Triggered**:
   - ExternalDNS hasn't reconciled yet
   - Wait: Default interval is 1 minute
   - Force: Restart ExternalDNS pod

#### Gateway Hostnames Not Resolving

**Symptom**: CNAME records exist, but no A records.

**Possible Causes**:

1. **IngressDNS Controller Not Running**:
   - IngressDNS controller creates DNSEndpoint CRDs with A records
   - Check: `kubectl get pods -n <operator-namespace> -l app=service-router-operator`
   - Fix: Ensure operator is running correctly

2. **Gateway Not Found**:
   - IngressDNS controller needs Gateway CRD to exist
   - Check: `kubectl get gateways.routing.router.io -A`
   - Fix: Create Gateway resource

3. **Service Not LoadBalancer**:
   - Gateway Service is ClusterIP or NodePort
   - IngressDNS controller cannot get LoadBalancer IP
   - Check: `kubectl get svc -n istio-system`
   - Fix: Change to `type: LoadBalancer`

4. **Load Balancer Not Provisioned**:
   - Service doesn't have external IP in status
   - Check: `kubectl get svc -n istio-system`
   - Fix: Check cloud provider integration

5. **DNSConfiguration Missing or Incorrect**:
   - IngressDNS controller reads ExternalDNS controllers from DNSConfiguration
   - Check: `kubectl get dnsconfiguration -A`
   - Fix: Ensure DNSConfiguration exists with correct controllers

#### Cross-Cluster Takeover Not Working

**Symptom**: New cluster cannot take over DNS records.

**Possible Causes**:

1. **Different Owner IDs**:
   - Cluster 1: `external-dns-weu`
   - Cluster 2: `external-dns-neu`
   - Fix: Use same owner ID (same region)

2. **Wrong Region**:
   - Trying to takeover cross-region
   - Fix: Takeover only works within same region

3. **Policy=upsert-only**:
   - ExternalDNS not deleting old records
   - Fix: Use `--policy=sync`

### Debugging Commands

#### List All DNS-Related Resources

```bash
# DNSEndpoints
kubectl get dnsendpoints -A -o wide

# DNSPolicies
kubectl get dnspolicies -A

# ServiceRoutes
kubectl get serviceroutes -A

# Gateways
kubectl get gateways.routing.router.io -A
```

#### Check Specific ServiceRoute Flow

```bash
# 1. Check ServiceRoute status
kubectl get serviceroute -n myapp api-route -o yaml

# 2. Check DNSPolicy status
kubectl get dnspolicy -n myapp -o yaml

# 3. Check generated DNSEndpoints
kubectl get dnsendpoints -n myapp -l router.io/serviceroute=api-route

# 4. Check ExternalDNS logs
kubectl logs -n external-dns -l app=external-dns-weu --tail=100
```

#### Test DNS Resolution

```bash
# From within cluster
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup api-ns-p-prod-myapp.aks.vecp.vczc.nl

# From host (if VNet peered)
nslookup api-ns-p-prod-myapp.aks.vecp.vczc.nl
```

### Useful ExternalDNS Flags for Debugging

```yaml
args:
  - --log-level=debug                    # Verbose logging
  - --dry-run                            # Don't actually create records
  - --once                               # Run once and exit (for testing)
  - --txt-prefix=test-                   # Use test prefix
```

## References

- [ExternalDNS GitHub](https://github.com/kubernetes-sigs/external-dns)
- [ExternalDNS Azure Provider](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/azure-private-dns.md)
- [DNSEndpoint CRD Spec](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/contributing/crd-source.md)
- [Service Router Operator Architecture](ARCHITECTURE.md)
