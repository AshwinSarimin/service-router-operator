# Architecture

Technical reference for the Service Router Operator architecture, controllers, CRDs, and DNS provisioning flow.

## Table of Contents

- [Architecture Diagram](#architecture-diagram)
- [Custom Resource Definitions](#custom-resource-definitions)
- [Controller Architecture](#controller-architecture)
- [DNS Name Format](#dns-name-format)
- [ExternalDNS Integration](#externaldns-integration)
- [Operational Modes](#operational-modes)
- [Security and RBAC](#security-and-rbac)

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
│  │          │ provides region,        │ provides DNS                   │  │
│  │          │ cluster, domain         │ controller list                │  │
│  │                                                                      │  │
│  │  Namespace-Scoped Controllers:                                      │  │
│  │  ┌──────────┐  ┌───────────┐  ┌──────────┐  ┌──────────────┐      │  │
│  │  │ Gateway  │──▶│ IngressDNS│  │DNSPolicy │  │ ServiceRoute │      │  │
│  │  │Controller│  │Controller │  │Controller│  │  Controller  │      │  │
│  │  └────┬─────┘  └─────┬─────┘  └────┬─────┘  └──────┬───────┘      │  │
│  │       │creates       │creates       │determines     │creates        │  │
│  │       │Istio Gateway │A records     │activeCtrlrs   │CNAME          │  │
│  └───────┼──────────────┼──────────────┼───────────────┼───────────────┘  │
│          │              │              │               │                  │
│     ┌────▼─────┐  ┌─────▼──────┐      │        ┌──────▼─────────┐        │
│     │  Istio   │  │ DNSEndpoint│      │        │  DNSEndpoint   │        │
│     │ Gateway  │  │(A records) │      │        │(CNAME records) │        │
│     └────┬─────┘  └─────┬──────┘      │        └──────┬─────────┘        │
│          │              └─────────────┼───────────────┘                  │
│          │                     ┌──────▼────────┐                         │
│          │                     │  ExternalDNS  │                         │
│          │                     └──────┬────────┘                         │
│     ┌────▼────────┐                   │                                  │
│     │   Service   │◄──────────────────┘ (monitors LoadBalancer IP)       │
│     │LoadBalancer │                                                       │
│     └────┬────────┘                                                       │
└──────────┼────────────────────────────────────────────────────────────────┘
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
    │ provides: region, cluster, domain, environmentLetter
    │
    ├──► Gateway (namespaced, reusable)
    │       ├──► Istio Gateway (generated)
    │       └──► DNSEndpoint A records (via IngressDNS Controller)
    │                 └──► Azure Private DNS A Records (via ExternalDNS)
    │
DNSConfiguration (cluster-scoped, singleton)
    │ defines: externalDNSControllers
    │
    └──► DNSPolicy (namespaced, per namespace)
            │ status: active, activeControllers
            │
            └──► ServiceRoute (namespaced, per service)
                    └──► DNSEndpoint CRDs (CNAME records, one per active controller)
                            └──► Azure Private DNS CNAME Records (via ExternalDNS)
```

> **Note**: The Gateway Controller aggregates hostnames from all ServiceRoutes into the Istio Gateway's `hosts` list. Users must create VirtualService resources separately to route traffic to their backends.

## Custom Resource Definitions

The operator defines five CRDs in two API groups. This separates cluster infrastructure (platform team) from namespace-level routing (application teams).

### API Groups

| API Group | CRDs | Managed By |
|-----------|------|------------|
| `cluster.router.io/v1alpha1` | ClusterIdentity, DNSConfiguration | Platform team |
| `routing.router.io/v1alpha1` | Gateway, DNSPolicy, ServiceRoute | Platform team (Gateway), App team (DNSPolicy, ServiceRoute) |

---

### ClusterIdentity

**Scope**: Cluster-wide singleton (must be named `cluster-identity`)

```yaml
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu                    # Geographic region code
  cluster: vec-weu-p-aks01       # Unique cluster identifier
  domain: aks.vecp.vczc.nl       # Base DNS domain
  environmentLetter: p           # Environment abbreviation (d/t/p)
  adoptsRegions: []              # Optional: orphan regions without K8s clusters
```

| Field | Description | Used For |
|-------|-------------|----------|
| `region` | Geographic region code (`weu`, `neu`, `frc`) | DNS routing, controller selection |
| `cluster` | Unique cluster identifier | Target hostname construction |
| `domain` | Base DNS domain | DNS record base |
| `environmentLetter` | Environment abbreviation | DNS hostname construction |
| `adoptsRegions` | Regions without K8s clusters this cluster manages | Active mode extension |

---

### DNSConfiguration

**Scope**: Cluster-wide singleton (must be named `dns-config`)

```yaml
apiVersion: cluster.router.io/v1alpha1
kind: DNSConfiguration
metadata:
  name: dns-config
spec:
  externalDNSControllers:
    - name: external-dns-weu    # Controller identifier (matches ExternalDNS deployment name)
      region: weu
    - name: external-dns-neu
      region: neu
    - name: external-dns-frc
      region: frc
```

The `name` field must match the annotation value ExternalDNS is configured to filter on. The `region` is used by DNSPolicy to determine which controllers are active for a given cluster.

---

### Gateway

**Scope**: Namespace-scoped (typically deployed in `istio-system`)

```yaml
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: default-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal  # Istio gateway pod selector
  credentialName: cert-aks-ingress               # TLS certificate secret name
  targetPostfix: internal                        # Gateway type identifier
```

| Field | Description |
|-------|-------------|
| `controller` | Istio ingress gateway pod selector (`spec.selector` in generated Istio Gateway) |
| `credentialName` | Kubernetes Secret containing TLS certificate |
| `targetPostfix` | Appended to gateway hostname: `{cluster}-{region}-{targetPostfix}.{domain}` |

The Gateway Controller generates an Istio `networking.istio.io/v1` Gateway resource with a dynamically aggregated `hosts` list built from all ServiceRoutes that reference this Gateway.

---

### DNSPolicy

**Scope**: Namespace-scoped (one per namespace)

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active           # Active or RegionBound
  sourceRegion: ""       # Optional: only activate when cluster region matches
  sourceCluster: ""      # Optional: only activate when cluster name matches
status:
  active: true
  activeControllers:
    - external-dns-weu
```

| Field | Description |
|-------|-------------|
| `mode` | `Active` = each cluster manages only its own region; `RegionBound` = one cluster manages all regions |
| `sourceRegion` | When set, policy is only active in the cluster matching this region |
| `sourceCluster` | When set, policy is only active in the cluster matching this name |
| `status.active` | Whether the policy is active in the current cluster |
| `status.activeControllers` | Which ExternalDNS controllers ServiceRoutes should target |

---

### ServiceRoute

**Scope**: Namespace-scoped (one per service)

```yaml
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: myapp
spec:
  serviceName: api                            # Kubernetes service name (used in DNS hostname)
  gatewayName: default-gateway               # Gateway resource to attach to
  gatewayNamespace: istio-system             # Namespace of the Gateway (cross-namespace supported)
  environment: prod                          # Environment segment for hostname
  application: myapp                         # Application segment for hostname
status:
  phase: Active                              # Pending, Active, or Failed
  dnsNames:
    - api-ns-p-prod-myapp.aks.vecp.vczc.nl
```

For each active ExternalDNS controller in `DNSPolicy.status.activeControllers`, the ServiceRoute Controller creates one DNSEndpoint CRD with a CNAME record pointing to the Gateway's target hostname.

## Controller Architecture

| Controller | Watches | Creates/Manages |
|------------|---------|-----------------|
| ClusterIdentity | ClusterIdentity CRD | In-memory cache (region, cluster, domain, environmentLetter) |
| DNSConfiguration | DNSConfiguration CRD | In-memory cache (controller list) |
| Gateway | Gateway CRD + LoadBalancer Services | Istio `networking.istio.io/v1` Gateway resources |
| IngressDNS | Gateway CRDs + Istio LoadBalancer Services | DNSEndpoint CRDs with A records for gateway hostnames |
| DNSPolicy | DNSPolicy CRD + ClusterIdentity + DNSConfiguration | Updates `status.active` and `status.activeControllers` |
| ServiceRoute | ServiceRoute CRD + DNSPolicy + Gateway + ClusterIdentity | DNSEndpoint CRDs with CNAME records |

All controllers use controller-runtime with leader election. Only one replica reconciles at a time; others are hot standby.

## DNS Name Format

### Service DNS Hostname

```
{serviceName}-ns-{environmentLetter}-{environment}-{application}.{domain}
```

**Example**: Service `api`, environment `prod`, application `myapp`, domain `aks.vecp.vczc.nl`, environmentLetter `p`:
```
api-ns-p-prod-myapp.aks.vecp.vczc.nl
```

This becomes the CNAME source, pointing to the gateway hostname.

### Gateway Target Hostname

```
{cluster}-{region}-{targetPostfix}.{domain}
```

**Example**: cluster `aks01`, region `weu`, targetPostfix `internal`, domain `aks.vecp.vczc.nl`:
```
aks01-weu-internal.aks.vecp.vczc.nl
```

This becomes the CNAME target and the A record name, resolving to the LoadBalancer IP.

### Complete DNS Chain

```
Client queries: api-ns-p-prod-myapp.aks.vecp.vczc.nl
    ↓  CNAME (ServiceRoute → DNSEndpoint)
    aks01-weu-internal.aks.vecp.vczc.nl
    ↓  A Record (IngressDNS Controller → DNSEndpoint)
    10.123.45.67  (LoadBalancer IP)
```

## ExternalDNS Integration

The operator creates `DNSEndpoint` CRDs (from `externaldns.k8s.io/v1alpha1`). ExternalDNS watches these and provisions records in Azure Private DNS. The operator does not write DNS records directly.

### Required ExternalDNS Configuration

```yaml
args:
  - --source=crd
  - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
  - --crd-source-kind=DNSEndpoint
  - --txt-owner-id=external-dns-weu                       # Must match region pattern
  - --txt-prefix=weu-p-aks01-                             # Unique prefix per cluster
  - --provider=azure-private-dns
  - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-weu
```

Each DNSEndpoint created by the operator carries:
- A `router.io/region` label matching the target ExternalDNS controller's region
- An `external-dns.alpha.kubernetes.io/controller` annotation matching the controller name

### Cross-Cluster DNS Takeover

Clusters in the same region share the same `--txt-owner-id`. When a failover cluster needs to take over DNS records, it can do so because ExternalDNS treats records with matching owner IDs as eligible for takeover.

This is a manual process — the operator does not perform automatic failover. Update the ServiceRoute or DNSPolicy to trigger the DNS change.

For complete ExternalDNS configuration details, see [ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md).

## Operational Modes

### Active Mode (default)

Each cluster manages DNS only for its own region. DNSPolicy is active in all clusters simultaneously; each cluster's ExternalDNS writes to its regional DNS zone.

| Cluster | Active | Active Controllers |
|---------|--------|--------------------|
| WEU | ✅ | `external-dns-weu` |
| NEU | ✅ | `external-dns-neu` |

Use when: regional data sovereignty, latency optimization, independent deployments.

### RegionBound Mode

One source cluster manages DNS for all regions. Other clusters deactivate their DNSPolicy and delete their DNSEndpoints to prevent conflicts.

```yaml
spec:
  mode: RegionBound
  sourceRegion: weu  # Only the WEU cluster's DNSPolicy will be active
```

| Cluster | Active | Active Controllers |
|---------|--------|--------------------|
| WEU | ✅ | `external-dns-weu`, `external-dns-neu`, `external-dns-frc` |
| NEU | ❌ | `[]` |

Use when: centralized DNS management, services without per-region deployments, regions without Kubernetes clusters.

### Active Mode with Adopted Regions

In Active mode, a cluster can adopt orphan regions (regions that have an ExternalDNS zone but no Kubernetes cluster) by adding `adoptsRegions` to ClusterIdentity:

```yaml
spec:
  adoptsRegions:
    - frc  # WEU cluster also manages DNS for the France Central zone
```

The WEU cluster's active controllers will then include `external-dns-frc`.

### DNSPolicy Inactive State

When a DNSPolicy is inactive (e.g., in RegionBound mode on a non-source cluster):

1. All DNSEndpoints for ServiceRoutes in that namespace are deleted
2. ServiceRoute status shows `phase: Pending`, `reason: DNSPolicyInactive`
3. ExternalDNS stops managing those DNS records

This prevents multiple clusters from racing to write the same DNS records.

## Security and RBAC

### Team Ownership

| Resource | Managed By | Scope |
|----------|-----------|-------|
| ClusterIdentity | Platform team | Cluster-wide |
| DNSConfiguration | Platform team | Cluster-wide |
| Gateway | Platform team | Namespace (typically `istio-system`) |
| DNSPolicy | Application team | Namespace |
| ServiceRoute | Application team | Namespace |

### Operator Service Account Permissions

The operator requires a ClusterRole with the following permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: service-router-operator
rules:
  - apiGroups: ["cluster.router.io", "routing.router.io"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["cluster.router.io", "routing.router.io"]
    resources: ["*/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["networking.istio.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["externaldns.k8s.io"]
    resources: ["dnsendpoints"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "watch"]
```

### Team RBAC Pattern

Platform teams need full access to `cluster.router.io` resources and `routing.router.io/gateways`. Application teams need access to `routing.router.io/dnspolicies` and `routing.router.io/serviceroutes` in their own namespaces, plus read access to Gateways.

---

## Related Documentation

- **[ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md)**: ExternalDNS configuration and DNS provisioning details
- **[Operator Guide](OPERATOR-GUIDE.md)**: Deployment, monitoring, and operational procedures
- **[User Guide](USER-GUIDE.md)**: Application team guide for DNSPolicy and ServiceRoute
- **[Installation Guide](INSTALLATION.md)**: Step-by-step deployment instructions
- **[Migration Guide](MIGRATION.md)**: Migrating from Helm chart to operator
