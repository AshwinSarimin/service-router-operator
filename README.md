# Service Router Operator

A Kubernetes operator for managing DNS records and Istio traffic routing across multi-cluster, multi-region deployments.

## Overview

The Service Router Operator simplifies multi-cluster, multi-region service deployments by managing DNS records and Istio Gateway configurations through Kubernetes-native custom resources.

**What it does**:
- **DNS Automation**: Automatically creates DNS records (CNAME and A records) for your services using ExternalDNS, eliminating manual DNS configuration
- **Gateway Management**: Aggregates service hostnames into Istio Gateway resources and manages DNS targets for Gateway LoadBalancer IPs
- **Regional Control**: Provides flexible DNS propagation strategies (Active mode for regional isolation, RegionBound mode for cross-region consolidation)
- **Namespace Isolation**: Enables application teams to manage their own DNS policies while platform teams control cluster-wide infrastructure

**The problem it solves**:

In multi-region Kubernetes deployments, exposing services requires coordinating:
1. DNS records across multiple regional DNS zones (e.g., Azure Private DNS per region)
2. Istio Gateway configurations with proper hostnames and TLS certificates
3. Regional routing strategies (serve traffic locally vs. centralized)
4. Team boundaries (platform vs. application team responsibilities)

Doing this manually or with static templates becomes error-prone, difficult to maintain, and doesn't adapt to changes automatically. This operator provides continuous reconciliation, ensuring your DNS and routing configuration always matches your desired state.

### Key Features

- **Multi-Region DNS Management**: Control DNS record propagation across regional DNS zones
- **Flexible Traffic Routing**: Active mode (regional isolation) or RegionBound mode (cross-region consolidation)
- **ExternalDNS Integration**: Automatic DNS provisioning via DNSEndpoint CRDs
- **Istio Gateway Automation**: Manages Istio Gateway resources with aggregated hostnames
- **Namespace Isolation**: DNS policies scoped per namespace for multi-tenancy
- **Declarative Configuration**: GitOps-friendly Kubernetes-native resources
- **Self-Healing**: Continuous reconciliation ensures desired state matches actual state

### Why an Operator?

The operator pattern provides continuous reconciliation and dynamic management that's ideal for multi-cluster DNS and routing:

| Traditional Approach | Operator Pattern |
|------------|----------|
| Static templates | Dynamic reconciliation |
| Manual updates | Automatic updates |
| Complex template logic | Native Kubernetes patterns |
| Limited multi-tenancy | Clear RBAC boundaries |
| One-time deployment | Continuous reconciliation |
| No status feedback | Real-time status updates |

**Key advantages**:
- **Self-Healing**: Automatically corrects configuration drift (e.g., if DNS records are manually deleted)
- **Dependency Awareness**: Updates DNS when Gateway IP addresses change
- **Validation**: Validates resources before applying them, catching errors early
- **Status Reporting**: Provides clear feedback on resource state (Active, Pending, Error)

**See [Architecture - Why an Operator?](docs/ARCHITECTURE.md#why-an-operator-instead-of-helm) for detailed rationale.**

## Quick Start

### Prerequisites

- Kubernetes 1.24+
- Istio 1.18+ (for Gateway resources)
- ExternalDNS 0.13+ (configured with DNSEndpoint CRD source)

### Installation

See [Installation Guide](docs/INSTALLATION.md) for detailed deployment instructions.

```bash
# Install CRDs (5 total)
kubectl apply -f config/crd/bases

# Install operator
kubectl apply -k config/default
```

### Basic Usage

**Platform Team** - Set up cluster infrastructure:

```bash
# 1. Create ClusterIdentity
kubectl apply -f config/samples/cluster_v1alpha1_clusteridentity.yaml

# 2. Create DNSConfiguration
kubectl apply -f config/samples/cluster_v1alpha1_dnsconfiguration.yaml

# 3. Create Gateway
kubectl apply -f config/samples/routing_v1alpha1_gateway.yaml
```

**Application Team** - Deploy your service:

```bash
# 4. Create DNSPolicy (per namespace)
kubectl apply -f config/samples/routing_v1alpha1_dnspolicy.yaml

# 5. Create ServiceRoute (per service)
kubectl apply -f config/samples/routing_v1alpha1_serviceroute.yaml

# 6. Create VirtualService (route traffic - operator does NOT create this)
kubectl apply -f your-virtualservice.yaml
```

**Result**: Your service DNS is provisioned and traffic routes correctly.

📘 **See [User Guide](docs/USER-GUIDE.md) for complete examples and detailed walkthrough.**

## Documentation

### For Platform Engineers

| Document | Description |
|----------|-------------|
| [**Architecture**](docs/ARCHITECTURE.md) | System design, CRD relationships, DNS flow, multi-region behavior |
| [**ExternalDNS Integration**](docs/EXTERNALDNS-INTEGRATION.md) | DNS provisioning, owner IDs, cross-cluster takeover |
| [**Operator Guide**](docs/OPERATOR-GUIDE.md) | Running and operating the controller, monitoring, troubleshooting |
| [**Installation**](docs/INSTALLATION.md) | Deployment procedures for homelab and AKS |
| [**Migration Guide**](docs/MIGRATION.md) | Migrating from Helm chart to operator |

### For Application Teams

| Document | Description |
|----------|-------------|
| [**User Guide**](docs/USER-GUIDE.md) | Using Gateway, DNSPolicy, and ServiceRoute CRDs |
| [**Architecture**](docs/ARCHITECTURE.md) | Understanding how the system works |

### For Contributors

| Document | Description |
|----------|-------------|
| [**Development Guide**](docs/DEVELOPMENT.md) | Contributing, development setup, testing |

## Custom Resources

The operator defines **5 CRDs across 2 API groups** with clear separation of concerns:

| CRD | API Group | Scope | Managed By |
|-----|-----------|-------|------------|
| **ClusterIdentity** | cluster.router.io/v1alpha1 | Cluster | Platform Team |
| **DNSConfiguration** | cluster.router.io/v1alpha1 | Cluster | Platform Team |
| **Gateway** | routing.router.io/v1alpha1 | Namespace | Platform Team |
| **DNSPolicy** | routing.router.io/v1alpha1 | Namespace | Application Team |
| **ServiceRoute** | routing.router.io/v1alpha1 | Namespace | Application Team |

### What the Operator Manages

✅ **Automatically Created**:
- **DNSEndpoint CRDs**: CNAME and A records for ExternalDNS
- **Istio Gateway resources**: With aggregated hostnames from ServiceRoutes

❌ **NOT Created** (user responsibility):
- **Istio VirtualService resources**: Users must create these to route traffic

### ClusterIdentity (Cluster-scoped)

Defines cluster metadata (region, cluster name, domain, environment) used for DNS record construction.

**Managed by**: Platform team | **Scope**: One per cluster

### DNSConfiguration (Cluster-scoped)

Defines available ExternalDNS controllers across the infrastructure, mapping controller names to regions.

**Managed by**: Platform team | **Scope**: One per cluster (singleton)

### Gateway (Namespace-scoped)

Wraps Istio Gateway configuration with DNS target information. The operator creates the Istio Gateway resource and aggregates hostnames from ServiceRoutes.

**Managed by**: Platform team | **Scope**: Typically in `istio-system`, shared across namespaces

### DNSPolicy (Namespace-scoped)

Defines DNS propagation strategy (Active or RegionBound mode) and determines which ExternalDNS controllers are active for services in the namespace.

**Managed by**: Application team | **Scope**: One per namespace (typically)

**Modes**:
- **Active**: Each cluster manages its own regional DNS
- **RegionBound**: One cluster manages DNS for multiple regions

### ServiceRoute (Namespace-scoped)

Links a Kubernetes service to a Gateway and triggers DNS record creation. Constructs DNS name from service, environment, and application fields.

**Managed by**: Application team | **Scope**: One per service

📘 **For complete CRD specifications and examples, see [Architecture](docs/ARCHITECTURE.md#custom-resource-definitions).**

## Architecture

### CRD Relationships

```
ClusterIdentity (cluster-wide)
    │
    │ provides: region, cluster, domain
    │
    ├──► Gateway (reusable)
    │       │
    │       └──► Istio Gateway (generated)
    │
DNSConfiguration (cluster-wide)
    │
    │ defines: externalDNSControllers
    │
    └──► DNSPolicy (per namespace)
            │
            │ determines: active controllers
            │
            └──► ServiceRoute (per service)
                    │
                    └──► DNSEndpoint CRDs (generated)
                            │
                            └──► DNS Records (via ExternalDNS)

Note: Istio Gateway is updated with hostnames from ServiceRoutes.
      Users must create VirtualService resources to route traffic.
```

### Network Flow

```
Client Query: api-ns-p-prod-myapp.example.com
    ↓
DNS: CNAME → aks01-weu-internal.example.com (created by operator)
    ↓
DNS: A record → 10.123.45.67 (created by operator via IngressDNS Controller)
    ↓
Load Balancer → Istio Gateway Pod
    ↓
Istio VirtualService (user-created, routes to service)
    ↓
Kubernetes Service → Backend Pod
```

📘 **For complete DNS flow and multi-region behavior, see [Architecture](docs/ARCHITECTURE.md#dns-and-network-flow).**

## Project Structure

```
.
├── api/                              # CRD type definitions
│   ├── cluster/v1alpha1/             # ClusterIdentity, DNSConfiguration
│   └── routing/v1alpha1/             # Gateway, DNSPolicy, ServiceRoute
├── cmd/
│   └── main.go                       # Application entry point
├── config/                           # Kubernetes manifests
│   ├── crd/bases/                    # Generated CRD YAML
│   ├── rbac/                         # RBAC configuration
│   ├── manager/                      # Operator deployment
│   └── samples/                      # Example custom resources
├── internal/
│   ├── controller/                   # Reconciliation logic
│   │   ├── cluster/                  # ClusterIdentity controller
│   │   └── routing/                  # DNSPolicy, Gateway, ServiceRoute
│   └── clusteridentity/              # In-memory cache
├── charts/service-router-operator/   # Helm chart for deployment
└── docs/                             # Documentation
    ├── ARCHITECTURE.md
    ├── EXTERNALDNS-INTEGRATION.md
    ├── OPERATOR-GUIDE.md
    ├── USER-GUIDE.md
    ├── MIGRATION.md
    ├── INSTALLATION.md
    └── DEVELOPMENT.md
```

## Use Cases

### Regional Service (Active Mode)

Each cluster manages DNS for its own region. Clients route to nearest cluster.

**Best For**:
- Latency-optimized routing
- Data sovereignty requirements
- Independent regional deployments

### Centralized Service (RegionBound Mode)

One cluster manages DNS for multiple regions. All clients route to central cluster.

**Best For**:
- Services in regions without clusters
- Cost optimization (fewer clusters)
- Centralized services (admin tools, databases)

### Multi-Gateway Routing

Different services use different gateways (internal, external).

**Best For**:
- Separating internal and public services
- Different TLS certificates per gateway type
- Security boundary enforcement

## Multi-Tenancy

Clear ownership boundaries enable multi-tenancy:

| Resource | Managed By | Scope |
|----------|------------|-------|
| ClusterIdentity | Platform Team | Cluster |
| DNSConfiguration | Platform Team | Cluster |
| Gateway | Platform Team | Shared (cross-namespace) |
| DNSPolicy | Application Team | Namespace |
| ServiceRoute | Application Team | Namespace |

**RBAC Support**: Platform and application teams have different permissions.

## Requirements

- **Kubernetes**: 1.24+
- **Istio**: 1.18+ (operator creates Gateway; users create VirtualService)
- **ExternalDNS**: 0.13+ (configured with `--source=crd`)
- **DNS Provider**: Azure Private DNS, AWS Route53, Google Cloud DNS, etc.

📘 **See [Installation Guide](docs/INSTALLATION.md) for complete prerequisites and setup.**

## Contributing

We welcome contributions! Please see our [Development Guide](docs/DEVELOPMENT.md).

This project follows:
- [Kubebuilder](https://book.kubebuilder.io/) best practices
- Go coding standards
- Kubernetes controller patterns

## License

Apache License 2.0

## Support

- **Documentation**: Start with [User Guide](docs/USER-GUIDE.md) or [Architecture](docs/ARCHITECTURE.md)
- **Issues**: [GitHub Issues](https://github.com/your-org/service-router-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/your-org/service-router-operator/discussions)
