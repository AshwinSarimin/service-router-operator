# Architecture

This document explains the technical architecture of the Service Router Operator, how it integrates with Istio and ExternalDNS, and the complete network flow from client request to backend service.

## Table of Contents

- [Overview](#overview)
- [Why an Operator Instead of Helm?](#why-an-operator-instead-of-helm)
- [Architecture Diagram](#architecture-diagram)
- [Custom Resource Definitions](#custom-resource-definitions)
- [Controller Architecture](#controller-architecture)
- [DNS and Network Flow](#dns-and-network-flow)
- [ExternalDNS Integration](#externaldns-integration)
- [Multi-Region Behavior](#multi-region-behavior)
- [Security and RBAC](#security-and-rbac)

## Overview

The Service Router Operator manages DNS records and Istio traffic routing for multi-cluster, multi-region Kubernetes deployments. It uses a **granular, multi-CRD architecture** that separates concerns and provides fine-grained control over DNS propagation and traffic routing.

### Key Features

- **Multi-Region DNS Management**: Control DNS record propagation across regional DNS zones
- **Flexible Traffic Routing**: Active mode (regional) or RegionBound mode (cross-region consolidation)
- **ExternalDNS Integration**: Automatic DNS provisioning via DNSEndpoint CRDs
- **Istio Gateway Automation**: Creates Istio Gateway resources with aggregated hosts from ServiceRoutes
- **Namespace Isolation**: DNS policies scoped per namespace for multi-tenancy
- **Declarative Configuration**: GitOps-friendly Kubernetes-native resources

### Problem Solved

In multi-region deployments, teams need to:
1. Route traffic to the correct regional cluster
2. Manage DNS records across multiple DNS zones (e.g., Azure Private DNS zones per region)
3. Support both regional isolation and cross-region consolidation scenarios
4. Maintain clear ownership boundaries between platform and application teams

The Service Router Operator automates this complexity through Kubernetes-native resources.

## How It Works

The operator uses Kubernetes controller pattern to:

1. **Watch** for changes to ServiceRoute, Gateway, DNSPolicy, and ClusterIdentity resources
2. **Generate** DNSEndpoint and Istio Gateway resources automatically
3. **Reconcile** continuously to ensure actual state matches desired state
4. **React** to changes in dependent resources (e.g., Gateway updates trigger ServiceRoute reconciliation)

This provides declarative, GitOps-friendly DNS and traffic routing management for multi-region deployments.

## Architecture Diagram

```
┌────────────────────────────────────────────────────────────────────────────┐
│                         Kubernetes Cluster (WEU)                           │
│                                                                            │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                    Service Router Operator                           │  │
│  │                                                                      │  │
│  │  Cluster-Scoped Controllers:                                        │  │
│  │  ┌───────────────┐        ┌────────────────┐                       │  │
│  │  │ ClusterIdentity│        │ DNSConfiguration│                       │  │
│  │  │  Controller   │        │   Controller    │                       │  │
│  │  └───────┬───────┘        └────────┬────────┘                       │  │
│  │          │                         │                                │  │
│  │          │ provides region,        │ provides DNS                   │  │
│  │          │ cluster, domain         │ controller list                │  │
│  │          │                         │                                │  │
│  │  ┌───────┴────────────────┬────────┴─────────┬──────────────────┐  │  │
│  │  │                        │                  │                  │  │  │
│  │  │  Namespace-Scoped Controllers:           │                  │  │  │
│  │  │                                           │                  │  │  │
│  │  ▼                        ▼                  ▼                  ▼  │  │
│  │  ┌──────────┐     ┌───────────┐     ┌──────────┐     ┌──────────┐  │  │
│  │  │ Gateway  │────▶│ IngressDNS│     │DNSPolicy │     │ServiceRoute│  │
│  │  │Controller│     │Controller │     │Controller│     │Controller│  │
│  │  └────┬─────┘     └─────┬─────┘     └────┬─────┘     └────┬─────┘  │  │
│  │       │                 │                 │                │        │  │
│  │       │creates          │creates          │determines      │creates │  │
│  │       │Istio Gateway    │Gateway A records│activeControllers│DNSEndpoints│  │
│  │       │with agg. hosts  │                 │                │(CNAME)     │  │
│  └───────┼─────────────────┼─────────────────┼────────────────┼────────┘  │
│          │                 │                 │                │           │
│     ┌────▼─────┐    ┌──────▼──────┐          │         ┌──────▼─────────┐ │
│     │  Istio   │    │ DNSEndpoint │          │         │  DNSEndpoint   │ │
│     │ Gateway  │    │(A records   │          │         │(CNAME records) │ │
│     │          │    │for Gateway) │          │         │                │ │
│     └────┬─────┘    └──────┬──────┘          │         └──────┬─────────┘ │
│          │                 │                 │                │           │
│          │                 └─────────────────┼────────────────┘           │
│          │                                   │                            │
│          │                            ┌──────▼────────┐                   │
│          │                            │  ExternalDNS  │                   │
│          │                            │   Controller  │                   │
│          │                            └──────┬────────┘                   │
│          │                                   │                            │
│     ┌────▼────────┐                          │                            │
│     │   Service   │                          │(monitors LoadBalancer)     │
│     │LoadBalancer │                          │                            │
│     └────┬────────┘                          │                            │
│          │                                   │                            │
└──────────┼───────────────────────────────────┼────────────────────────────┘
           │                                   │
           │ (Azure Load Balancer)             │
           │                                   │
           └───────────┬───────────────────────┘
                       │
                 ┌─────▼──────┐
                 │  Azure     │
                 │  Private   │
                 │  DNS       │
                 └────────────┘
```

### Resource Relationships

```
ClusterIdentity (cluster-scoped, singleton)
    │
    │ provides: region, cluster, domain, environmentLetter
    │
    ├──► Gateway (namespaced, reusable)
    │       │
    │       │ defines: controller, credentialName, targetPostfix
    │       │
    │       ├──► Istio Gateway (generated)
    │       │
    │       └──► DNSEndpoint (A records for Gateway, via IngressDNS Controller)
    │                 │
    │                 └──► Azure Private DNS A Records (via ExternalDNS)
    │
DNSConfiguration (cluster-scoped, singleton)
    │
    │ defines: externalDNSControllers (all available controllers)
    │
    └──► DNSPolicy (namespaced, per team/namespace)
            │
            │ defines: mode, sourceRegion, sourceCluster
            │ status:  active, activeControllers (from DNSConfiguration)
            │
            └──► ServiceRoute (namespaced, per service)
                    │
                    │ defines: serviceName, gatewayName, environment, application
                    │
                    └──► DNSEndpoint CRDs (CNAME records, one per active controller)
                            │
                            └──► Azure Private DNS CNAME Records (via ExternalDNS)

Note: Gateway Controller aggregates hostnames from all ServiceRoutes
      and adds them to the Istio Gateway's hosts list.
      Users must create VirtualService resources separately to route traffic.
```

## Custom Resource Definitions

The operator defines five custom resources organized into two API groups. This separation enables clear ownership boundaries and RBAC scoping.

### 1. ClusterIdentity (Cluster-scoped)

**API Group**: `cluster.router.io/v1alpha1`  
**Purpose**: Identifies the region, cluster, and domain configuration for the entire cluster  
**Scope**: Cluster-wide (single instance per cluster)  
**Managed By**: Platform team

#### Specification

```yaml
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity  # Must be named "cluster-identity"
spec:
  region: weu                    # Regional identifier (neu, weu, frc)
  cluster: vec-weu-d-aks01       # Cluster identifier
  domain: aks.vecd.vczc.nl       # Base domain for DNS records
  environmentLetter: d           # Environment (d=dev, t=test, p=prod)
  adoptsRegions: []              # Optional: orphan regions this cluster adopts
status:
  phase: Active                  # Pending, Active, or Failed
  conditions:
    - type: Ready
      status: "True"
      reason: ValidationSucceeded
      message: ClusterIdentity is valid
```

#### Fields

| Field | Description | Example | Used For |
|-------|-------------|---------|----------|
| `region` | Geographic region code | `weu` | DNS routing, controller selection |
| `cluster` | Unique cluster identifier | `vec-weu-d-aks01` | Target hostname construction |
| `domain` | Base DNS domain | `aks.vecd.vczc.nl` | DNS record base |
| `environmentLetter` | Environment abbreviation | `d` (dev) | DNS hostname construction |
| `adoptsRegions` | List of orphan regions (without K8s clusters) this cluster should manage DNS for | `["frc"]` | Used in **Active mode** to extend DNS management to regions without deployed clusters |

#### Behavior

- Validates cluster identity configuration
- Populates in-memory cache for fast access by other controllers
- Updates status based on validation results
- Single source of truth for cluster metadata

#### Examples

```yaml
# Production West Europe cluster
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu
  cluster: vec-weu-p-aks01
  domain: aks.vecp.vczc.nl
  environmentLetter: p
```

This configuration tells the operator: *"This is production cluster 01 in West Europe, using the vecp domain."*

```yaml
# Production West Europe cluster adopting France Central region
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu
  cluster: vec-weu-p-aks01
  domain: aks.vecp.vczc.nl
  environmentLetter: p
  adoptsRegions:
    - frc  # This cluster manages DNS for FRC zone (no K8s cluster in FRC)
```

This configuration tells the operator: *"This is production cluster 01 in West Europe, also managing DNS for France Central which has no deployed cluster."*

---

### 2. DNSConfiguration (Cluster-scoped)

**API Group**: `cluster.router.io/v1alpha1`  
**Purpose**: Defines all ExternalDNS controllers available in the infrastructure  
**Scope**: Cluster-wide (single instance per cluster, singleton)  
**Managed By**: Platform team

**Key Concepts**:
- **Cluster-scoped singleton**: Only one DNSConfiguration per cluster (typically named `dns-config`)
- **Prerequisite for DNSPolicy**: DNSPolicy reads from this to determine active controllers
- **Controller registry**: Lists all ExternalDNS instances with their region assignments
- **Platform-managed**: Created by platform team during cluster setup

#### Specification

```yaml
apiVersion: cluster.router.io/v1alpha1
kind: DNSConfiguration
metadata:
  name: dns-config  # Singleton name
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
    - name: external-dns-frc
      region: frc
status:
  conditions:
    - type: Ready
      status: "True"
      reason: ConfigurationValid
      message: DNSConfiguration is valid
```

#### Fields

| Field | Description | Required | Example |
|-------|-------------|----------|---------|
| `externalDNSControllers` | List of all ExternalDNS controllers | Yes | See below |
| `externalDNSControllers[].name` | Controller identifier (logical name) | Yes | `external-dns-weu` |
| `externalDNSControllers[].region` | Geographic region for this controller | Yes | `weu` |

**Note**: 
- `name` must match the controller name annotation DNSEndpoint resources will target
- `region` is used by DNSPolicy to filter controllers based on cluster region (Active mode)

#### Behavior

1. **Validation**: Ensures at least one controller is defined and all have unique names
2. **Cache Population**: Makes controller list available to DNSPolicy controller
3. **Status Updates**: Marks configuration as Ready when valid

#### Examples

```yaml
# Multi-region infrastructure with 3 DNS zones
apiVersion: cluster.router.io/v1alpha1
kind: DNSConfiguration
metadata:
  name: dns-config
spec:
  externalDNSControllers:
    - name: external-dns-weu          # West Europe DNS zone
      region: weu
    - name: external-dns-neu          # North Europe DNS zone
      region: neu
    - name: external-dns-frc          # France Central DNS zone
      region: frc                      # (no cluster, but DNS zone exists)
```

This configuration tells the operator: *"There are three ExternalDNS controllers managing regional DNS zones. WEU cluster will use external-dns-weu in Active mode, but can use all three in RegionBound mode."*

---

### 3. Gateway (Namespace-scoped)

**API Group**: `routing.router.io/v1alpha1`  
**Purpose**: Defines reusable Istio Gateway infrastructure  
**Scope**: Namespaced (typically deployed in `istio-system`)  
**Managed By**: Platform team

#### Specification

```yaml
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: default-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal  # Istio gateway selector
  credentialName: cert-aks-ingress               # TLS certificate secret
  targetPostfix: internal                        # Gateway type identifier
status:
  phase: Active
  loadBalancerIP: 10.123.45.67                   # External IP of gateway service
  conditions:
    - type: Ready
      status: "True"
      reason: GatewayCreated
      message: Istio Gateway created successfully
```

#### Fields

| Field | Description | Example | Used For |
|-------|-------------|---------|----------|
| `controller` | Istio gateway pod selector | `aks-istio-ingressgateway-internal` | Istio Gateway `.spec.selector` |
| `credentialName` | K8s secret with TLS cert | `cert-aks-ingress` | Istio Gateway TLS configuration |
| `targetPostfix` | Gateway type identifier | `internal`, `external`, `gateway` | Target hostname construction |

#### Hostname Pattern

**Format**: `{cluster}-{region}-{targetPostfix}.{domain}`

**Example**: 
- ClusterIdentity: `cluster=aks01, region=weu, domain=example.com`
- Gateway: `targetPostfix=internal`
- **Result**: `aks01-weu-internal.example.com`

This hostname is:
1. Used as the target in DNSEndpoint CNAME records
2. Resolved to the Gateway LoadBalancer IP via DNSEndpoint A records (created by IngressDNS Controller)

#### Generated Resources

The controller creates an Istio Gateway:

```yaml
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: default-gateway
  namespace: istio-system
  ownerReferences:
    - apiVersion: routing.router.io/v1alpha1
      kind: Gateway
      name: default-gateway
spec:
  selector:
    istio: aks-istio-ingressgateway-internal
  servers:
    - port:
        number: 443
        name: https
        protocol: HTTPS
      tls:
        mode: SIMPLE
        credentialName: cert-aks-ingress
      hosts:
        - "api-ns-d-dev-myapp.aks.vecd.vczc.nl"
        - "web-ns-d-dev-myapp.aks.vecd.vczc.nl"
```

**Note**: The `hosts` list is dynamically populated by aggregating all ServiceRoute hostnames that reference this Gateway. The operator never generates wildcard hosts; only specific FQDNs from ServiceRoutes are added.

#### Examples

```yaml
---
# Internal gateway for services within VNet
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: internal-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal
  credentialName: cert-aks-ingress-internal
  targetPostfix: internal
---
# External gateway for public-facing services
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: external-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-external
  credentialName: cert-aks-ingress-external
  targetPostfix: external
```

---

### 4. DNSPolicy (Namespace-scoped)

**API Group**: `routing.router.io/v1alpha1`  
**Purpose**: Defines DNS propagation strategy for a namespace  
**Scope**: Namespaced (one per namespace or application team)  
**Managed By**: Application team (with platform guidance)

**Key Concepts**:
- **Filters controllers from DNSConfiguration**: Does not define controllers itself
- **Determines active controllers**: Via `status.activeControllers` field
- **Prerequisites**: DNSConfiguration must exist cluster-wide
- **Namespace-scoped**: Each namespace can have different DNS behavior
- **Status-driven**: ServiceRoute reads `status.activeControllers` to create DNSEndpoints

#### Specification

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active                              # Active or RegionBound
  sourceRegion: ""                          # Optional: only activate in matching region
  sourceCluster: ""                         # Optional: only activate in matching cluster
status:
  phase: Active                             # Pending, Active, Failed, Inactive
  active: true                              # Is this policy active?
  activeControllers:                        # Controllers active for this cluster (from DNSConfiguration)
    - external-dns-weu
  conditions:
    - type: Ready
      status: "True"
      reason: PolicyActive
      message: DNSPolicy is active for this cluster
```

#### Fields

| Field | Description | Required | Example |
|-------|-------------|----------|---------|
| `mode` | DNS management strategy | Yes | `Active`, `RegionBound` |
| `sourceRegion` | Region filter for policy activation | No | `weu` |
| `sourceCluster` | Cluster filter for policy activation | No | `vec-weu-d-aks01` |
| `status.phase` | Current phase of policy | N/A (status) | `Active`, `Inactive` |
| `status.active` | Is policy active in this cluster? | N/A (status) | `true`/`false` |
| `status.activeControllers` | Which controllers are active? | N/A (status) | `["external-dns-weu"]` |

#### Mode Behavior

##### Active Mode (Regional Isolation)

**Concept**: Each cluster manages DNS **only for its own region**.

**Behavior**:
- Policy is active (unless `sourceRegion`/`sourceCluster` filters don't match)
- Controllers from DNSConfiguration matching the cluster's region are activated
- **Additionally**, if ClusterIdentity has `adoptsRegions` defined, controllers for those regions are also activated
- Each cluster independently manages its regional DNS zone (and any adopted regions)
- DNS records point to the local cluster's gateway

**Example**:

Assume **DNSConfiguration** exists with:
```yaml
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
```

**DNSPolicy** in Active mode:
```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
```

**In WEU Cluster** (ClusterIdentity region=weu):
- `status.active`: `true`
- `status.activeControllers`: `["external-dns-weu"]` (filtered to match cluster region)
- DNSEndpoint created with controller annotation: `external-dns-weu`
- DNS record: `api.example.com` → `aks01-weu-internal.example.com`

**In NEU Cluster** (ClusterIdentity region=neu):
- `status.active`: `true`
- `status.activeControllers`: `["external-dns-neu"]` (filtered to match cluster region)
- DNSEndpoint created with controller annotation: `external-dns-neu`
- DNS record: `api.example.com` → `aks02-neu-internal.example.com`

**Traffic Flow**:
```
WEU Clients → WEU DNS Zone → aks01-weu-internal.example.com → WEU Cluster
NEU Clients → NEU DNS Zone → aks02-neu-internal.example.com → NEU Cluster
```

**Use Cases**:
- Regional data sovereignty requirements
- Latency-optimized routing (users route to nearest region)
- Independent regional deployments
- **Serving traffic from orphan regions** (regions without Kubernetes clusters, using ClusterIdentity `adoptsRegions`)

##### Active Mode with Adopted Regions (Orphan Region Support)

**Concept**: A cluster manages DNS for **its own region + orphan regions** that have no Kubernetes clusters.

**Behavior**:
- ClusterIdentity defines `adoptsRegions` field listing orphan regions
- Controllers for cluster's region + adopted regions are activated
- DNS records for adopted regions point to the adopting cluster's gateway
- Enables serving traffic from regions without deploying infrastructure

**Example**:

Assume **DNSConfiguration** exists with:
```yaml
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-neu
      region: neu
    - name: external-dns-frc
      region: frc
```

**ClusterIdentity** in WEU cluster adopts FRC (France Central has no K8s cluster):
```yaml
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu
  cluster: vec-weu-p-aks01
  domain: aks.vecp.vczc.nl
  environmentLetter: p
  adoptsRegions:
    - frc  # WEU cluster manages DNS for FRC zone
```

**DNSPolicy** in Active mode:
```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
```

**In WEU Cluster** (ClusterIdentity region=weu, adoptsRegions=[frc]):
- `status.active`: `true`
- `status.activeControllers`: `["external-dns-weu", "external-dns-frc"]` (own region + adopted)
- DNSEndpoints created with controller annotations: `external-dns-weu` and `external-dns-frc`
- WEU DNS record: `api.example.com` → `aks01-weu-internal.example.com`
- FRC DNS record: `api.example.com` → `aks01-weu-internal.example.com` (points to WEU cluster)

**In NEU Cluster** (ClusterIdentity region=neu, no adopted regions):
- `status.active`: `true`
- `status.activeControllers`: `["external-dns-neu"]`
- NEU DNS record: `api.example.com` → `aks02-neu-internal.example.com`

**Traffic Flow**:
```
WEU Clients → WEU DNS Zone → aks01-weu-internal.example.com → WEU Cluster
FRC Clients → FRC DNS Zone → aks01-weu-internal.example.com → WEU Cluster (adopted region)
NEU Clients → NEU DNS Zone → aks02-neu-internal.example.com → NEU Cluster
```

**Use Cases**:
- Serving traffic from regions without deploying Kubernetes clusters
- Gradual regional expansion (adopt region initially, deploy cluster later)
- Cost optimization for low-traffic regions

##### RegionBound Mode (Cross-Region Consolidation)

**Concept**: One cluster manages DNS for **multiple regions**, routing all traffic to itself.

**Behavior**:
- Policy only activates in clusters matching `sourceRegion` (and optionally `sourceCluster`)
- Once active, **ALL** controllers from DNSConfiguration are activated (regardless of their region)
- DNS records in all DNS zones point to the source cluster's gateway
- Other clusters with the same DNSPolicy have `status.active=false` and create nothing

**Example**:

Assume **DNSConfiguration** exists with:
```yaml
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-frc
      region: frc
    - name: external-dns-neu
      region: neu
```

**DNSPolicy** in RegionBound mode:
```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Only activate in WEU cluster
```

**In WEU Cluster** (ClusterIdentity region=weu, **matches sourceRegion**):
- `status.active`: `true`
- `status.activeControllers`: `["external-dns-weu", "external-dns-frc", "external-dns-neu"]` (all controllers)
- DNSEndpoints created for all three controllers
- DNS records in WEU zone: `api.example.com` → `aks01-weu-internal.example.com`
- DNS records in FRC zone: `api.example.com` → `aks01-weu-internal.example.com`
- DNS records in NEU zone: `api.example.com` → `aks01-weu-internal.example.com`

**In NEU Cluster** (ClusterIdentity region=neu, **doesn't match sourceRegion**):
- `status.active`: `false`
- `status.activeControllers`: `[]`
- **No DNSEndpoints created**
- No DNS records provisioned

**Traffic Flow**:
```
WEU Clients → WEU DNS Zone → aks01-weu-internal.example.com → WEU Cluster
FRC Clients → FRC DNS Zone → aks01-weu-internal.example.com → WEU Cluster (cross-region)
NEU Clients → NEU DNS Zone → aks01-weu-internal.example.com → WEU Cluster (cross-region)
```

**Use Cases**:
- Cost optimization (consolidate to fewer clusters for centralized services)
- Disaster recovery scenarios (manually redirect traffic by updating DNSPolicy)
- Preventing cross-cluster DNS conflicts (only one cluster writes records)

**Important**: This is **not automatic failover**. DNS changes require manual DNSPolicy updates.

#### Behavior

1. **Read DNSConfiguration**: Retrieves list of all available controllers
2. **Activation Check**: Compares `sourceRegion`/`sourceCluster` (if set) against ClusterIdentity
3. **Set Status**: Updates `status.active` (`true` or `false`)
4. **Mode Logic**:
   - **Active mode**: Filter controllers from DNSConfiguration to only those matching cluster's region (plus any regions listed in ClusterIdentity `adoptsRegions`)
   - **RegionBound mode**: If policy is active, use **all** controllers from DNSConfiguration
5. **Populate Status**: Sets `status.activeControllers` with filtered list
6. **ServiceRoute Integration**: ServiceRoute controller reads `status.activeControllers` to determine which DNSEndpoints to create

#### Examples

```yaml
---
# Example 1: Regional service (available in all deployed regions)
# Requires DNSConfiguration with weu and neu controllers
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: frontend-dns
  namespace: frontend
spec:
  mode: Active
# activeControllers populated from DNSConfiguration based on cluster region
---
# Example 2: Centralized service (only WEU serves, but all regions can reach it)
# Requires DNSConfiguration with weu and neu controllers
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: admin-dns
  namespace: admin
spec:
  mode: RegionBound
  sourceRegion: weu
# In WEU cluster: activeControllers = [all controllers from DNSConfiguration]
# In NEU cluster: activeControllers = [] (policy inactive)
---
# Example 3: Conditional activation (only activate in specific cluster)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: migration-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu
  sourceCluster: vec-weu-p-aks01  # Only activate in this specific cluster
```

---

### 5. ServiceRoute (Namespace-scoped)

**API Group**: `routing.router.io/v1alpha1`  
**Purpose**: Per-service configuration linking applications to gateways and DNS  
**Scope**: Namespaced  
**Managed By**: Application team

#### Specification

```yaml
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: myapp
spec:
  serviceName: api                          # Service name for DNS
  gatewayName: default-gateway              # Reference to Gateway
  gatewayNamespace: istio-system            # Gateway namespace (optional)
  environment: prod                         # Environment name
  application: myapp                        # Application name
status:
  phase: Active
  DNSEndpoint: api-route-external-dns-weu   # First generated DNSEndpoint
  conditions:
    - type: Ready
      status: "True"
      reason: ReconciliationSucceeded
      message: ServiceRoute is active
```

#### Fields

| Field | Description | Required | Example |
|-------|-------------|----------|---------|
| `serviceName` | Service name component | Yes | `api` |
| `gatewayName` | Reference to Gateway CRD | Yes | `default-gateway` |
| `gatewayNamespace` | Gateway namespace | No (default: `istio-system`) | `istio-system` |
| `environment` | Environment name | Yes | `prod`, `dev`, `test` |
| `application` | Application name | Yes | `myapp` |

#### Hostname Construction

**Source Hostname** (what clients use):

**Format**: `{serviceName}-ns-{envLetter}-{environment}-{application}.{domain}`

**Example**:
- `serviceName`: `api`
- `envLetter`: `p` (from ClusterIdentity)
- `environment`: `prod`
- `application`: `myapp`
- `domain`: `example.com` (from ClusterIdentity)
- **Result**: `api-ns-p-prod-myapp.example.com`

**Target Hostname** (gateway entry point):

**Format**: `{cluster}-{region}-{gatewayPostfix}.{domain}`

**Example**:
- `cluster`: `aks01` (from ClusterIdentity)
- `region`: `weu` (from ClusterIdentity)
- `gatewayPostfix`: `internal` (from Gateway)
- `domain`: `example.com` (from ClusterIdentity)
- **Result**: `aks01-weu-internal.example.com`

#### Generated Resources

**Important**: ServiceRoute only creates DNS records (DNSEndpoint resources). It does NOT create Istio VirtualService resources. Application teams must create their own VirtualService resources to route traffic to backend services.

##### DNSEndpoint (one per active controller)

Instructs ExternalDNS to create DNS records:

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
spec:
  endpoints:
    # CNAME record: application hostname → gateway hostname
    - dnsName: api-ns-p-prod-myapp.example.com
      recordType: CNAME
      targets:
        - aks01-weu-internal.example.com
```

**Key Points**:
- One DNSEndpoint per active controller (from DNSPolicy status)
- Controller annotation must match ExternalDNS `--annotation-filter`
- ExternalDNS automatically creates TXT records for ownership tracking based on its `--txt-owner-id` configuration (enables cross-cluster DNS takeover when owner IDs match)
- OwnerReference ensures automatic cleanup when ServiceRoute is deleted

#### Behavior

**Reconciliation Steps**:

1. **Fetch ServiceRoute** from API server
2. **Validate** specification (required fields, format)
3. **Fetch DNSPolicy** in same namespace
4. **Check if DNSPolicy is active** (based on `status.active`)
5. **Fetch Gateway** (referenced by name)
6. **Fetch ClusterIdentity** (cluster singleton)
7. **Generate DNSEndpoints** (one per `status.activeControllers`)
8. **Apply resources** (create/update/delete as needed)
9. **Update status** (Ready condition, first DNSEndpoint name)
10. **Gateway Controller** watches ServiceRoutes and aggregates hostnames into Istio Gateway

**Note**: ServiceRoute does NOT create VirtualService resources. Application teams must create VirtualService resources separately.

**Cross-Resource Watches**:

The controller watches related resources to trigger reconciliation:

- **DNSPolicy** changes → reconcile all ServiceRoutes in namespace
- **Gateway** changes → reconcile all ServiceRoutes referencing it
- **ClusterIdentity** changes → reconcile all ServiceRoutes cluster-wide

#### Examples

```yaml
---
# Example 1: Simple API service
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: myapp
spec:
  serviceName: api
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: myapp
# Result: api-ns-p-prod-myapp.aks.vecp.vczc.nl → aks01-weu-internal.aks.vecp.vczc.nl
---
# Example 2: Internal admin service
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: admin-route
  namespace: myapp
spec:
  serviceName: admin
  gatewayName: internal-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: myapp
# Result: admin-ns-p-prod-myapp.aks.vecp.vczc.nl → aks01-weu-internal.aks.vecp.vczc.nl
---
# Example 3: Multiple services, same gateway
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: auth-route
  namespace: identity
spec:
  serviceName: auth
  gatewayName: default-gateway
  environment: dev
  application: identity
---
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: pep-route
  namespace: identity
spec:
  serviceName: pep
  gatewayName: default-gateway
  environment: dev
  application: identity
```

## Controller Architecture

### Multi-Controller Design

The operator consists of six controllers, each responsible for specific CRDs or infrastructure:

1. **ClusterIdentity Controller** (`internal/controller/cluster/`)
   - Validates cluster identity configuration
   - Populates in-memory cache
   - Updates status

2. **DNSConfiguration Controller** (`internal/controller/cluster/`)
   - Validates DNS controller configuration
   - Populates in-memory cache for DNSPolicy lookups
   - Updates status

3. **Gateway Controller** (`internal/controller/routing/`)
   - Generates Istio Gateway resources
   - Watches ClusterIdentity for domain changes

4. **DNSPolicy Controller** (`internal/controller/routing/`)
   - Reads controller list from DNSConfiguration
   - Determines active controllers based on mode and filters
   - Updates status with activation state and activeControllers list
   - Watches ClusterIdentity for region changes

5. **IngressDNS Controller** (`internal/controller/routing/`)
   - Manages DNS A records for Gateway target hostnames
   - Creates DNSEndpoint resources for gateway LoadBalancer IPs
   - Watches Gateway CRDs, Services, ClusterIdentity, and DNSConfiguration
   - Performs garbage collection of orphaned DNSEndpoints
   - Completes DNS resolution chain: CNAME → A record → Gateway IP

6. **ServiceRoute Controller** (`internal/controller/routing/`)
   - Generates DNSEndpoint (CNAME records) only
   - Reads `status.activeControllers` from DNSPolicy to create appropriate DNSEndpoints
   - Watches DNSPolicy, Gateway, and ClusterIdentity
   - Does NOT create VirtualService resources (user responsibility)
   - Gateway Controller aggregates ServiceRoute hostnames into Istio Gateway

### Reconciliation Loop

**ServiceRoute Controller Example**:

```
┌──────────────────────────────────────────────────────────────┐
│                  Reconciliation Trigger                       │
│  (ServiceRoute created/updated, or watched resource changed)  │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  1. Fetch ServiceRoute from API Server                      │
│     - Check if deleted (cleanup DNSEndpoints if so)         │
│     - Validate spec (serviceName, gatewayName, etc.)        │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  2. Fetch DNSPolicy in same namespace                       │
│     - Check if DNSPolicy exists                             │
│     - Check status.active (is policy active?)               │
│     - If inactive: Update ServiceRoute status to Pending    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  3. Fetch referenced Gateway                                │
│     - Namespace from spec.gatewayNamespace (or default)     │
│     - If not found: Update ServiceRoute status to Failed    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  4. Fetch ClusterIdentity (singleton)                       │
│     - Try cache first (fast path)                           │
│     - Fall back to API if cache empty                       │
│     - If not found: Update ServiceRoute status to Pending   │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  5. Generate DNSEndpoints                                   │
│     - One per activeControllers from DNSPolicy.status       │
│     - Build source hostname (client-facing)                 │
│     - Build target hostname (gateway)                       │
│     - Add controller annotation and owner ID                │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  6. Reconcile DNSEndpoints                                  │
│     - List existing DNSEndpoints (by labels)                │
│     - Create missing, update changed, delete stale          │
│     - Gateway Controller separately aggregates hostnames    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  7. Update ServiceRoute Status                              │
│     - Set Ready condition to True                           │
│     - Store first DNSEndpoint name                          │
│     - Set observedGeneration                                │
└─────────────────────────────────────────────────────────────┘

Note: Users must create VirtualService resources separately.
      Gateway Controller adds this ServiceRoute's hostname to Istio Gateway.
```

### Resource Ownership and Lifecycle

**Ownership Pattern**:

```
ServiceRoute (user-created)
    │
    └──► DNSEndpoint (operator-created)
            │
            └─ ownerReference: ServiceRoute
               ↓
            (deleted when ServiceRoute is deleted)

Gateway (user-created)
    │
    └──► Istio Gateway (operator-created)
            │
            ├─ ownerReference: Gateway
            │
            └─ hosts: aggregated from all ServiceRoutes
               ↓
            (deleted when Gateway is deleted)

VirtualService (user-created manually)
    │
    ├─ hosts: service hostnames
    ├─ gateways: references Istio Gateway
    └─ routes traffic to backend services
```

**Key Benefits**:
- **Automatic Cleanup**: Deleting a ServiceRoute automatically deletes its DNSEndpoints
- **Clear Ownership**: OwnerReferences make it obvious which resources are derived
- **Garbage Collection**: Kubernetes handles cleanup, no manual intervention needed
- **Separation of Concerns**: DNS management (operator) vs. routing rules (application teams)

### Cross-Resource Watches

Controllers set up watches to react to changes in related resources:

```go
// ServiceRoute controller watches:
func (r *ServiceRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&ServiceRoute{}).                                    // Primary resource
        Owns(&externaldnsv1alpha1.DNSEndpoint{}).                // Owned resource
        Watches(&DNSPolicy{}, mapToDependentServiceRoutes()).    // Related resource (same namespace)
        Watches(&Gateway{}, mapToDependentServiceRoutes()).      // Related resource (referenced)
        Watches(&ClusterIdentity{}, mapToAllServiceRoutes()).    // Related resource (cluster-wide)
        Complete(r)
}
```

**What Triggers Reconciliation**:

| Change | Affected ServiceRoutes | Reason |
|--------|----------------------|---------|
| ServiceRoute created/updated | That ServiceRoute | Direct change |
| DNSEndpoint created/updated/deleted | Owning ServiceRoute | Owned resource changed |
| DNSPolicy updated | All ServiceRoutes in namespace | Active controllers might have changed |
| Gateway updated | All ServiceRoutes referencing it | Target hostname might have changed |
| ClusterIdentity updated | **All ServiceRoutes cluster-wide** | Region/domain/cluster metadata changed |

### Cluster Identity Cache

To avoid repeated API calls, the operator maintains an in-memory cache of ClusterIdentity:

```go
// Package: internal/clusteridentity

type ClusterIdentity struct {
    Region            string
    Cluster           string
    Domain            string
    EnvironmentLetter string
}

var cache *ClusterIdentity
var cacheMutex sync.RWMutex

func Get() *ClusterIdentity {
    cacheMutex.RLock()
    defer cacheMutex.RUnlock()
    return cache
}

func Set(ci *ClusterIdentity) {
    cacheMutex.Lock()
    defer cacheMutex.Unlock()
    cache = ci
}
```

**Usage Pattern**:

```go
// Helper function used by all controllers
func getClusterIdentity(ctx context.Context, client client.Client) (*ClusterIdentity, error) {
    // 1. Try cache first (fast path)
    if cached := clusteridentity.Get(); cached != nil {
        return cached, nil
    }

    // 2. Fall back to API (slow path)
    var ci clusterv1alpha1.ClusterIdentity
    if err := client.Get(ctx, types.NamespacedName{Name: "cluster-identity"}, &ci); err != nil {
        return nil, err
    }

    // 3. Populate cache for next time
    cached := &clusteridentity.ClusterIdentity{
        Region:            ci.Spec.Region,
        Cluster:           ci.Spec.Cluster,
        Domain:            ci.Spec.Domain,
        EnvironmentLetter: ci.Spec.EnvironmentLetter,
    }
    clusteridentity.Set(cached)

    return cached, nil
}
```

**Benefits**:
- **Performance**: Avoids API calls on every reconciliation
- **Consistency**: All controllers use same cached value
- **Automatic Update**: ClusterIdentity controller refreshes cache on changes

## DNS and Network Flow

### Complete Request Flow

This section explains the entire path from a client DNS query to a response from the backend service.

```
┌────────────┐
│  Client    │
│ (Browser,  │
│  App, etc) │
└─────┬──────┘
      │ 1. DNS Query: api-ns-p-prod-myapp.aks.vecp.vczc.nl
      │
      ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Azure Private DNS Zone                         │
│                  (e.g., aks.vecp.vczc.nl)                       │
│                                                                  │
│  CNAME Record:                                                   │
│    api-ns-p-prod-myapp.aks.vecp.vczc.nl                        │
│    → aks01-weu-internal.aks.vecp.vczc.nl                       │
│                                                                  │
│  A Record:                                                       │
│    aks01-weu-internal.aks.vecp.vczc.nl                         │
│    → 10.123.45.67 (Istio Gateway Load Balancer IP)             │
└─────┬───────────────────────────────────────────────────────────┘
      │ 2. DNS Response: 10.123.45.67
      │
      ▼
┌────────────────────────────────────────────────────────────────┐
│           Azure Load Balancer (Internal or Public)             │
│                     IP: 10.123.45.67                           │
│                                                                 │
│  Forwards traffic to Istio Gateway pods                        │
└─────┬──────────────────────────────────────────────────────────┘
      │ 3. HTTPS Request: Host: api-ns-p-prod-myapp.aks.vecp.vczc.nl
      │
      ▼
┌────────────────────────────────────────────────────────────────┐
│               Istio Gateway Pod                                 │
│         (aks-istio-ingressgateway-internal)                     │
│                                                                 │
│  1. TLS Termination (using cert-aks-ingress secret)            │
│  2. SNI Routing (based on Host header)                         │
│  3. Match Gateway hosts (from ServiceRoutes)                   │
│  4. Forward to VirtualService (user-created)                   │
└─────┬──────────────────────────────────────────────────────────┘
      │ 4. Match VirtualService
      │
      ▼
┌────────────────────────────────────────────────────────────────┐
│         Istio VirtualService (USER-CREATED)                     │
│                                                                 │
│  hosts:                                                         │
│    - api-ns-p-prod-myapp.aks.vecp.vczc.nl                     │
│  gateways:                                                      │
│    - istio-system/default-gateway                              │
│  http:                                                          │
│    - route:                                                     │
│        - destination:                                           │
│            host: api.myapp.svc.cluster.local                   │
└─────┬──────────────────────────────────────────────────────────┘
      │ 5. Route to service
      │
      ▼
┌────────────────────────────────────────────────────────────────┐
│           Kubernetes Service (api.myapp)                        │
│                                                                 │
│  Selects backend pods via label selectors                      │
└─────┬──────────────────────────────────────────────────────────┘
      │ 6. Load balance to pod
      │
      ▼
┌────────────────────────────────────────────────────────────────┐
│                  Backend Pod                                    │
│                                                                 │
│  Application container processes request                        │
└─────┬──────────────────────────────────────────────────────────┘
      │ 7. Response
      │
      ▼
┌────────────┐
│  Client    │
│  receives  │
│  response  │
└────────────┘
```

### DNS Record Creation Flow

How DNS records are created from a ServiceRoute:

```
┌─────────────────────────────────────────────────────────────────┐
│  1. User creates ServiceRoute                                   │
│                                                                  │
│     apiVersion: routing.router.io/v1alpha1                      │
│     kind: ServiceRoute                                          │
│     metadata:                                                   │
│       name: api-route                                           │
│       namespace: myapp                                          │
│     spec:                                                       │
│       serviceName: api                                          │
│       gatewayName: default-gateway                              │
│       environment: prod                                         │
│       application: myapp                                        │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. ServiceRoute Controller Reconciles                          │
│                                                                  │
│     - Fetches DNSPolicy (finds active controllers)              │
│     - Fetches Gateway (gets targetPostfix)                      │
│     - Fetches ClusterIdentity (gets region, cluster, domain)    │
│     - Generates DNSEndpoint for each active controller          │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. DNSEndpoint Created                                         │
│                                                                  │
│     apiVersion: externaldns.k8s.io/v1alpha1                     │
│     kind: DNSEndpoint                                           │
│     metadata:                                                   │
│       name: api-route-external-dns-weu                          │
│       namespace: myapp                                          │
│       annotations:                                              │
│         external-dns.alpha.kubernetes.io/controller:            │
│           external-dns-weu                                      │
│     spec:                                                       │
│       endpoints:                                                │
│         - dnsName: api-ns-p-prod-myapp.aks.vecp.vczc.nl        │
│           recordType: CNAME                                     │
│           targets:                                              │
│             - aks01-weu-internal.aks.vecp.vczc.nl              │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. ExternalDNS Controller Watches DNSEndpoint                  │
│                                                                  │
│     - Filters by annotation (external-dns-weu)                  │
│     - Reads spec.endpoints                                      │
│     - Connects to Azure Private DNS                             │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. ExternalDNS Creates DNS Records in Azure                    │
│                                                                  │
│     Azure Private DNS Zone: aks.vecp.vczc.nl                    │
│                                                                  │
│     CNAME Record:                                               │
│       api-ns-p-prod-myapp.aks.vecp.vczc.nl                     │
│       → aks01-weu-internal.aks.vecp.vczc.nl                    │
│                                                                  │
│     TXT Record (ownership):                                     │
│       weu-p-aks01-_external-dns-owner.api-ns-p...              │
│       → "heritage=external-dns,                                 │
│          external-dns/owner=external-dns-weu,                   │
│          external-dns/resource=api-ns-p..."                     │
└─────────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. Gateway Hostname Resolution (IngressDNS Controller)         │
│                                                                  │
│     IngressDNS Controller watches Gateways and Services:        │
│       - Aggregates all Gateway targetPostfix values             │
│       - Finds LoadBalancer Service for each Gateway controller  │
│       - Gets LoadBalancer IP from Service status                │
│                                                                  │
│     Creates DNSEndpoint with A record:                          │
│       name: gateway-controller-<controller>-<postfix>-<dns>     │
│       dnsName: aks01-weu-internal.aks.vecp.vczc.nl             │
│       recordType: A                                             │
│       targets: [10.123.45.67]  (LoadBalancer IP)               │
│                                                                  │
│     One DNSEndpoint per ExternalDNS controller defined in       │
│     DNSConfiguration (enables multi-region DNS propagation)     │
└─────────────────────────────────────────────────────────────────┘
```

### Multi-Region DNS Resolution

How clients in different regions resolve the same hostname:

#### Active Mode (Regional DNS Zones)

```
Client in WEU:
    1. Query: api.example.com
    2. DNS Resolver: Uses WEU Private DNS Zone
    3. CNAME: api.example.com → aks01-weu-gateway.example.com
    4. A Record: aks01-weu-gateway.example.com → 10.1.2.3 (WEU cluster)
    5. Traffic: Routed to WEU cluster

Client in NEU:
    1. Query: api.example.com
    2. DNS Resolver: Uses NEU Private DNS Zone
    3. CNAME: api.example.com → aks02-neu-gateway.example.com
    4. A Record: aks02-neu-gateway.example.com → 10.4.5.6 (NEU cluster)
    5. Traffic: Routed to NEU cluster

Result: Regional traffic stays regional (latency optimization)
```

#### RegionBound Mode (Cross-Region Routing)

```
Client in WEU:
    1. Query: api.example.com
    2. DNS Resolver: Uses WEU Private DNS Zone
    3. CNAME: api.example.com → aks01-weu-gateway.example.com
    4. A Record: aks01-weu-gateway.example.com → 10.1.2.3 (WEU cluster)
    5. Traffic: Routed to WEU cluster

Client in NEU:
    1. Query: api.example.com
    2. DNS Resolver: Uses NEU Private DNS Zone
    3. CNAME: api.example.com → aks01-weu-gateway.example.com  ← Same as WEU!
    4. A Record: aks01-weu-gateway.example.com → 10.1.2.3 (WEU cluster)
    5. Traffic: Routed to WEU cluster (cross-region)

Result: All traffic goes to WEU cluster (centralized)
```

## ExternalDNS Integration

The operator relies on ExternalDNS to provision actual DNS records in Azure Private DNS (or other DNS providers). This section explains the integration in detail.

**For a comprehensive guide, see [ExternalDNS Integration Documentation](EXTERNALDNS-INTEGRATION.md).**

### Quick Overview

**What ExternalDNS Does**:
- Watches DNSEndpoint CRDs created by the operator
- Creates/updates/deletes DNS records in Azure Private DNS
- Automatically creates TXT records for ownership tracking (based on `--txt-owner-id` configuration) to enable cross-cluster DNS takeover

**How the Operator Integrates**:
1. **Creates DNSEndpoint CRDs** with CNAME records and proper controller annotations
2. **ExternalDNS automatically adds TXT records** for ownership tracking based on its `--txt-owner-id` configuration
3. **Labels resources** for tracking and filtering

**Key Configuration Requirements**:

ExternalDNS must be deployed with:

```yaml
args:
  - --source=crd                                         # Watch CRDs
  - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
  - --crd-source-kind=DNSEndpoint
  - --txt-owner-id=external-dns-weu                      # Must match region pattern
  - --txt-prefix=weu-p-aks01-                            # Unique prefix per cluster
  - --provider=azure-private-dns
  - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-weu
```

### Gateway Hostname Resolution

The operator creates CNAME records pointing to gateway hostnames (e.g., `aks01-weu-internal.example.com`). How do these resolve to IPs?

**Answer**: The IngressDNS Controller creates DNSEndpoint resources with A records.

#### How IngressDNS Controller Works

The IngressDNS Controller reconciles DNS A records for Gateway infrastructure by:

1. **Watching Gateway CRDs**: Tracks all Gateway resources to know which target hostnames are needed
2. **Watching LoadBalancer Services**: Monitors Istio Gateway Services to get their LoadBalancer IPs
3. **Creating DNSEndpoint Resources**: Creates DNSEndpoint CRDs with A records for each Gateway's target hostname
4. **Multi-Region Support**: Creates one DNSEndpoint per ExternalDNS controller (from DNSConfiguration)

#### Example Flow

When a Gateway resource exists:

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

The IngressDNS Controller:

1. Finds the LoadBalancer Service matching the controller:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: aks-istio-ingressgateway-internal
  namespace: istio-system
spec:
  type: LoadBalancer
status:
  loadBalancer:
    ingress:
      - ip: 10.123.45.67
```

2. Creates DNSEndpoint resources (one per ExternalDNS controller):
```yaml
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: gateway-controller-aks-istio-ingressgateway-internal-internal-external-dns-weu
  namespace: istio-system
  labels:
    router.io/istio-controller: aks-istio-ingressgateway-internal
    router.io/target-postfix: internal
    router.io/resource-type: gateway-service
spec:
  endpoints:
    - dnsName: aks01-weu-internal.aks.vecp.vczc.nl
      recordType: A
      targets:
        - 10.123.45.67
```

3. ExternalDNS processes this DNSEndpoint and creates the A record in Azure Private DNS

#### Complete DNS Chain

```
Client queries: api-ns-p-prod-myapp.aks.vecp.vczc.nl
    ↓
CNAME (from ServiceRoute → DNSEndpoint CRD):
    api-ns-p-prod-myapp.aks.vecp.vczc.nl
    → aks01-weu-internal.aks.vecp.vczc.nl
    ↓
A Record (from IngressDNS Controller → DNSEndpoint CRD):
    aks01-weu-internal.aks.vecp.vczc.nl
    → 10.123.45.67 (Load Balancer IP)
    ↓
Client connects to 10.123.45.67
```

**Key Point**: The operator does NOT rely on Service annotations. All DNS records are managed through DNSEndpoint CRDs.

### Cross-Cluster DNS Takeover

The operator enables **cross-cluster DNS takeover** through shared owner IDs.

#### How It Works

**Scenario**: Two clusters in the same region (both WEU)

**Cluster 1** (aks01):
- ExternalDNS `--txt-owner-id=external-dns-weu`
- ExternalDNS `--txt-prefix=weu-p-aks01-`
- Creates DNS record: `api.example.com` → `aks01-weu-gateway.example.com`
- ExternalDNS automatically creates TXT record: `weu-p-aks01-api.example.com` → `"external-dns-weu"`

**Cluster 2** (aks02):
- ExternalDNS `--txt-owner-id=external-dns-weu` (same!)
- ExternalDNS `--txt-prefix=weu-p-aks02-`
- Sees existing TXT record with owner `external-dns-weu`
- **Can take ownership** because owner ID matches
- Updates DNS record: `api.example.com` → `aks02-weu-gateway.example.com`
- ExternalDNS creates new TXT record: `weu-p-aks02-api.example.com` → `"external-dns-weu"`

#### Use Cases

1. **Manual Failover**: If aks01 fails, manually update ServiceRoute or DNSPolicy so aks02 takes over DNS records
2. **Maintenance**: Drain aks01, update DNS configuration to point to aks02
3. **Blue-Green Deployments**: Switch traffic between clusters by updating ServiceRoute configuration

#### Important Notes

- **Same Region Only**: Cross-cluster takeover only works within the same region (same owner ID)
- **Manual Process**: The operator does NOT automatically detect failures or perform failover; you must explicitly update ServiceRoute or DNSPolicy configuration
- **No Health Checking**: The operator does not monitor cluster health or automatically switch traffic
- **TXT Record Cleanup**: Old TXT records remain; use `--policy=sync` to clean them up

## Multi-Region Behavior

### Active Mode vs. RegionBound Mode Comparison

| Aspect | Active Mode | RegionBound Mode |
|--------|-------------|------------------|
| **DNS Scope** | Each cluster manages only its region | One cluster manages multiple regions |
| **Controller Selection** | Only matching cluster's region | All configured regions |
| **Traffic Pattern** | Regional (clients route locally) | Centralized (clients route cross-region) |
| **Use Case** | Latency optimization, data sovereignty | Cost optimization, regions without clusters |
| **DNS Management** | Distributed (per-region) | Centralized (single source cluster) |
| **Policy Activation** | Active in all clusters (by default) | Active only in `sourceRegion` cluster |

### Region Change Behavior

**Changing from Active to RegionBound**:

```yaml
# Before (Active mode)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active

# After (RegionBound mode)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Only WEU cluster is active
```

**What Happens**:

1. **WEU Cluster**:
   - DNSPolicy status changes: `active: true`, `activeControllers: [external-dns-weu, external-dns-neu]`
   - ServiceRoute controller sees new activeControllers
   - Creates DNSEndpoint for both `external-dns-weu` and `external-dns-neu`
   - ExternalDNS in WEU creates records in both WEU and NEU DNS zones

2. **NEU Cluster**:
   - DNSPolicy status changes: `active: false`, `activeControllers: []`
   - ServiceRoute controller sees policy is inactive
   - Deletes all DNSEndpoints in namespace
   - ExternalDNS in NEU stops managing records (will clean up if `--policy=sync`)

3. **DNS Resolution**:
   - WEU DNS zone: `api.example.com` → `aks01-weu-gateway.example.com` (WEU cluster)
   - NEU DNS zone: `api.example.com` → `aks01-weu-gateway.example.com` (WEU cluster)
   - NEU clients now route to WEU cluster (cross-region traffic)

### Cross-Region Controller Configuration

**Scenario**: You have a DNS zone in FRC region, but no Kubernetes cluster there.

**Solution**: Configure DNSConfiguration with an FRC controller, then use RegionBound mode in WEU.

**DNSConfiguration** (cluster-scoped, created by platform team):
```yaml
apiVersion: cluster.router.io/v1alpha1
kind: DNSConfiguration
metadata:
  name: dns-config
spec:
  externalDNSControllers:
    - name: external-dns-weu
      region: weu
    - name: external-dns-frc   # Controller for FRC DNS zone
      region: frc              # Logical region identifier
```

**DNSPolicy** (namespaced, created by app team):
```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Only activate in WEU
```

**Explanation**:
- DNSConfiguration defines `external-dns-frc` controller with region `frc`
- DNSPolicy in RegionBound mode activates only in WEU cluster
- When active, DNSPolicy populates `status.activeControllers` with ALL controllers from DNSConfiguration
- WEU cluster creates DNSEndpoint with annotation `external-dns.alpha.kubernetes.io/controller: external-dns-frc`
- An ExternalDNS controller named `external-dns-frc` (deployed in WEU) watches for this annotation and creates records in the FRC DNS zone

### DNSPolicy Lifecycle and DNSEndpoint Management

#### Active vs Inactive State

A DNSPolicy can be in one of two states:

- **Active** (`Status.Active = true`): The policy applies to the current cluster
  - ServiceRoutes create DNSEndpoints
  - external-dns controllers process the endpoints
  - DNS records are provisioned

- **Inactive** (`Status.Active = false`): The policy does not apply to the current cluster
  - ServiceRoutes delete all their DNSEndpoints
  - No DNS records are managed by this cluster
  - Prevents race conditions with external-dns controllers

#### When Does a DNSPolicy Become Inactive?

In **RegionBound mode**, a DNSPolicy becomes inactive when:
- `spec.sourceRegion` is set and doesn't match the cluster's region
- `spec.sourceCluster` is set and doesn't match the cluster's name

This ensures only ONE cluster manages DNS records across all regions, while other clusters remain dormant.

**Example**:

```yaml
# DNSPolicy in WEU Cluster
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Matches WEU cluster's region
status:
  active: true  # ✅ Active in WEU
  activeControllers:
    - external-dns-weu
    - external-dns-neu
    - external-dns-frc
```

```yaml
# Same DNSPolicy in NEU Cluster
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Does NOT match NEU cluster's region
status:
  active: false  # ❌ Inactive in NEU
  activeControllers: []  # No controllers active
```

#### Race Condition Prevention

When a DNSPolicy transitions from Active → Inactive:

1. **ServiceRoute controller detects the status change**
2. **Immediately deletes all DNSEndpoints** for that ServiceRoute
3. **Updates ServiceRoute status** to Pending with reason "DNSPolicyInactive"
4. **external-dns controllers stop processing** (no endpoints to process)
5. **DNS record conflicts are avoided**

This prevents the scenario where:
- Cluster A manages DNS in RegionBound mode (active)
- Cluster B has an inactive DNSPolicy
- Without cleanup: Cluster B's external-dns would still process stale endpoints
- Result: Both clusters compete to write DNS records → race condition

**Reconciliation Flow**:

```
┌─────────────────────────────────────────────────────────────┐
│ DNSPolicy.Status.Active changes from true → false          │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│ ServiceRoute Controller                                     │
│  1. Detects DNSPolicy.Status.Active = false                │
│  2. Calls deleteDNSEndpointsForServiceRoute()              │
│  3. Lists DNSEndpoints with matching labels                │
│  4. Deletes each DNSEndpoint                               │
│  5. Updates ServiceRoute.Status.Phase = Pending            │
│  6. Sets Condition.Reason = DNSPolicyInactive              │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│ ExternalDNS Controller                                      │
│  - No more DNSEndpoints to process                         │
│  - Cleans up DNS records (if --policy=sync)                │
│  - No race condition with other clusters                   │
└─────────────────────────────────────────────────────────────┘
```

#### Idempotency and Edge Cases

The cleanup mechanism is **idempotent** and handles edge cases:

**Case 1: ServiceRoute created when DNSPolicy already inactive**
- ServiceRoute reconcile detects inactive policy
- Calls deletion function (no DNSEndpoints exist yet)
- Deletion function returns success (empty list is valid)
- ServiceRoute status set to Pending
- No errors, graceful handling

**Case 2: DNSEndpoints already deleted**
- Subsequent reconciliations call deletion function
- Function lists endpoints, gets empty list
- Returns success immediately
- No API errors, fully idempotent

**Case 3: Rapid DNSPolicy state changes**
- Each reconciliation is independent
- Controller converges to desired state
- No race conditions between reconciliations

## Security and RBAC

### RBAC Model

The operator uses a **least-privilege RBAC model** with clear separation between platform and application teams.

#### Platform Team Permissions

Platform teams manage cluster-wide and infrastructure resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: service-router-platform-admin
rules:
  # Cluster-wide resources
  - apiGroups: ["cluster.router.io"]
    resources: ["clusteridentities"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Gateway infrastructure (typically in istio-system)
  - apiGroups: ["routing.router.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Istio Gateway resources (created by operator)
  - apiGroups: ["networking.istio.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Note: VirtualServices are NOT created by the operator
  # Add virtualservices permissions if platform team needs to manage them manually:
  # - apiGroups: ["networking.istio.io"]
  #   resources: ["virtualservices"]
  #   verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

#### Application Team Permissions

Application teams manage namespace-scoped resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: service-router-app-user
  namespace: myapp
rules:
  # DNS policies for namespace
  - apiGroups: ["routing.router.io"]
    resources: ["dnspolicies"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Service routes for applications
  - apiGroups: ["routing.router.io"]
    resources: ["serviceroutes"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Read-only access to status
  - apiGroups: ["routing.router.io"]
    resources: ["dnspolicies/status", "serviceroutes/status"]
    verbs: ["get", "list", "watch"]
  
  # Read-only access to Gateways (to reference them)
  - apiGroups: ["routing.router.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch"]
```

### Multi-Tenancy Considerations

#### Namespace Isolation

- **DNSPolicy**: Scoped to namespace, team controls their own DNS strategy
- **ServiceRoute**: Scoped to namespace, team controls their own services
- **Gateway**: Can be shared across namespaces (referenced by name)

#### Cross-Namespace References

ServiceRoute can reference a Gateway in a different namespace:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: team-a
spec:
  serviceName: api
  gatewayName: shared-gateway
  gatewayNamespace: istio-system  # Cross-namespace reference
  environment: prod
  application: team-a
```

**RBAC Requirement**: The team needs read access to Gateways in `istio-system`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-a-gateway-reader
  namespace: istio-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: gateway-reader
subjects:
  - kind: ServiceAccount
    name: team-a-sa
    namespace: team-a
```

### Security Best Practices

1. **Limit ClusterIdentity Access**: Only platform admins should modify ClusterIdentity
2. **Gateway Reuse**: Create shared Gateways in `istio-system`, grant read-only access to teams
3. **DNSPolicy per Namespace**: Each team manages their own DNS strategy
4. **ServiceRoute Ownership**: Teams only manage ServiceRoutes in their namespaces
5. **Audit Logging**: Enable Kubernetes audit logs to track changes to CRDs

### Operator Service Account Permissions

The operator itself needs broad permissions to manage resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: service-router-operator
rules:
  # CRDs (all namespaces)
  - apiGroups: ["cluster.router.io", "routing.router.io"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Status updates
  - apiGroups: ["cluster.router.io", "routing.router.io"]
    resources: ["*/status"]
    verbs: ["get", "update", "patch"]
  
  # Istio Gateway resources (operator creates these)
  - apiGroups: ["networking.istio.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # ExternalDNS CRDs
  - apiGroups: ["externaldns.k8s.io"]
    resources: ["dnsendpoints"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Services (for Gateway LoadBalancer IP detection)
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "watch"]
```

---

## Related Documentation

- **[ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md)**: Detailed guide on ExternalDNS configuration and DNS provisioning
- **[Operator Guide](OPERATOR-GUIDE.md)**: Running and operating the controller
- **[User Guide](USER-GUIDE.md)**: Using the CRDs (Gateway, DNSPolicy, ServiceRoute)
- **[Installation Guide](INSTALLATION.md)**: Deployment procedures for homelab and AKS
- **[Migration Guide](MIGRATION.md)**: Migrating from Helm chart to operator
- **[Development Guide](DEVELOPMENT.md)**: Contributing and development setup

---

## Appendix: Design Rationale

### Why Five CRDs?

**Separation of Concerns**: Each CRD has a distinct responsibility:
1. **ClusterIdentity**: Cluster-wide identity and metadata (singleton per cluster)
2. **DNSConfiguration**: ExternalDNS controller registry (singleton per cluster)
3. **Gateway**: Reusable Istio gateway infrastructure (shared across namespaces)
4. **DNSPolicy**: Namespace-level DNS propagation strategy (one per team/namespace)
5. **ServiceRoute**: Per-service DNS and routing configuration (one per workload)

**Multi-Tenancy Benefits**:
- Platform teams manage ClusterIdentity, DNSConfiguration, and Gateway
- Application teams manage DNSPolicy and ServiceRoute in their namespaces
- Clear ownership boundaries and RBAC scoping

**Reusability**:
- Gateway definitions are shared across multiple ServiceRoutes
- Avoids duplicating gateway configuration per service

**Flexibility**:
- Easy to add new modes or features without breaking existing resources
- Independent versioning of different concerns

### Why Multi-Group API Layout?

**Logical Grouping**:
- `cluster.router.io`: Cluster-level configuration (ClusterIdentity, DNSConfiguration)
- `routing.router.io`: All routing and DNS resources (Gateway, DNSPolicy, ServiceRoute)

**RBAC Granularity**:

```yaml
# Example: Allow team to manage routing and DNS but not cluster config
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: app-team-role
rules:
  - apiGroups: ["routing.router.io"]
    resources: ["serviceroutes", "dnspolicies"]
    verbs: ["*"]
  # No access to cluster.router.io (ClusterIdentity, DNSConfiguration)
  - apiGroups: ["routing.router.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch"]  # Read-only for Gateway
```

### Why Namespace-Scoped Gateway?

While Gateway could be cluster-scoped, namespace-scoped provides:

1. **Flexibility**: Multiple teams can define their own gateways if needed
2. **Isolation**: Gateway lifecycle tied to namespace
3. **Standard Pattern**: Istio Gateway resources are namespace-scoped
4. **RBAC**: Namespace-level permissions work naturally
5. **Reusability**: Gateways can still be shared via cross-namespace references
