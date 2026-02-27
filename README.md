# Multi region networking

![multi-region](../../assets/images/docs/networking/multi-region.drawio.png)

The AKS platform operates across multiple Azure regions (North Europe and West Europe) to provide high availability, disaster recovery capabilities, and geographic proximity to users. In this multi-region architecture, a critical requirement is the ability to route traffic seamlessly between regions. When an application or service becomes unavailable in one region, whether due to maintenance, failure, or regional issues, traffic must be automatically or manually redirected to a healthy instance in another region. This cross-region routing capability is essential for:

- **High Availability**: Ensuring services remain accessible even when a regional deployment fails
- **Disaster Recovery**: Enabling failover scenarios during regional outages
- **Maintenance Windows**: Redirecting traffic during planned maintenance without service disruption
- **Geographic Optimization**: Routing users to the nearest or best-performing region

Managing DNS across multiple regions presents significant operational complexity: workload teams must route traffic to the correct regional cluster, maintain DNS records across multiple Azure Private DNS zones, handle both regional isolation and cross-region failover scenarios, and prevent conflicts when multiple controllers attempt to manage the same DNS records. Traditional approaches require manual, error-prone DNS management that doesn't scale.

## Service Router Operator

To address these challenges, the [Service Router Operator](https://dev.azure.com/vecozo/Platform/_git/Aks.Operator.ServiceRouter) provides automated DNS management for multi-region deployments:

- **Automates DNS Record Creation**: Automatically generates DNS records based on Custom Resources
- **Enables Regional Control**: Supports both Active (multi-region) and RegionBound (single-region) operational modes
- **Prevents Conflicts**: Uses label-based filtering to ensure each region's ExternalDNS instance only manages its designated DNS records
- **Simplifies Operations**: Workload teams simply declare their services and desired routing behavior using Helm charts

## How it works

The Service Router automates DNS provisioning and traffic routing for services deployed across multiple AKS clusters and regions.

### Components

**Service Router Operator**

The operator manages the lifecycle of DNS records by continuously reconciling Custom Resource Definitions (CRDs) to ensure the desired state matches the actual state.

**ExternalDNS**

ExternalDNS is an AKS platform component that automatically synchronizes Kubernetes networking resources (Services, Ingresses, and DNSEndpoint CRs) with DNS providers like Azure Private DNS zones. It monitors DNSEndpoint custom resources created by the Service Router Operator and provisions the corresponding DNS records.

**Key Architecture**: Each AKS cluster runs multiple ExternalDNS instances, one for each regional DNS zone. This enables seamless failover when a cluster becomes unavailable, as the healthy cluster's ExternalDNS instance can take over DNS management for the failed region. 

**Label-Based Filtering**

The Service Router Operator uses labels to ensure region-specific ExternalDNS instances only manage their designated DNS records. When a DNSEndpoint is created with label `app: external-dns-neu`, only the NEU ExternalDNS instance processes it and creates records in the NEU Private DNS zone. This ensures:

- Prevention of DNS conflicts between regions
- Independent region management
- Clear ownership and responsibility
- Support for both Active and RegionBound operational modes

**Important**: The Service Router Operator does not directly create DNS records. It creates `DNSEndpoint` Custom Resources that ExternalDNS watches and uses to provision DNS records in Azure Private DNS.

By combining the Service Router Operator with ExternalDNS and Azure Private DNS, the platform achieves fully automated, conflict-free DNS management that enables seamless traffic routing between regions.

### DNS Provisioning Workflow

The operator consists of multiple controllers that continuously reconcile the defined Custom Resource Definitions (CRDs) to ensure the desired state matches the actual state. All CRDs are declared in GitOps repositories and reconciled with Flux.

![service-router-components](../../assets/images/docs/networking/service-router-components.png)

**Configuration Responsibilities:**

1. **Platform Team** - Configures platform-level CRDs (ClusterIdentity, DNSConfiguration, Gateway)
   - See [Custom Resource Definitions](https://dev.azure.com/vecozo/Platform/_git/Aks.Operator.ServiceRouter?path=/docs/ARCHITECTURE.md&anchor=custom-resource-definitions) for details.

2. **Workload Team** - Configures workload-level CRDs:
   - **ServiceRoute**: Links a service to a Gateway and DNS hostname
   - **DNSPolicy**: Defines the operational mode (Active or RegionBound) for a namespace

Workload teams can easily configure these CRDs using the [aks-webapp-charts](https://dev.azure.com/vecozo/Platform/_git/Aks.Platform?path=/charts/service/aks-webapp-charts) Helm chart.

**Complete Workflow**:

1. Workload team deploys application with ServiceRoute and DNSPolicy
2. Service Router Operator reconciles CRDs and creates DNSEndpoint resources
3. ExternalDNS watches DNSEndpoint resources
4. ExternalDNS provisions CNAME records in Azure Private DNS zones
5. DNS records point to the appropriate Istio Ingress Gateway based on the operational mode

For a detailed step-by-step guide, see the [User Guide](https://dev.azure.com/vecozo/Platform/_git/Aks.Operator.ServiceRouter?path=%2Fdocs%2FUSER-GUIDE.md&version=GBmain&_a=preview).

### Operational Modes

The Service Router supports two operational modes that determine how DNS records are managed across regions:

- **Active Mode**: Each cluster manages DNS records for its own region (Active-Active deployment)
- **RegionBound Mode**: One cluster manages DNS records for multiple regions (Active-Passive deployment)

#### Choosing the Right Mode

| Requirement | Recommended Mode |
|-------------|------------------|
| Workload deployed in multiple regions | **Active** |
| Low latency for regional users | **Active** |
| Data residency or compliance requirements | **Active** |
| Workload deployed in single region only | **RegionBound** |
| Cost optimization (fewer deployments) | **RegionBound** |
| Centralized data access (single database) | **RegionBound** |

#### Active Mode

Use **Active Mode** when your workload is deployed in multiple regions. Regional users will use local traffic routing (low latency) with regional isolation and independent failover.

![Active-mode](../../assets/images/docs/networking/Mode-Active.png)

- NEU cluster creates: `app1.aks.veczd.vczc.nl` → `aks01-neu-ingressgateway-internal.aks.veczd.vczc.nl`
- WEU cluster creates: `app1.aks.veczd.vczc.nl` → `aks01-weu-ingressgateway-internal.aks.veczd.vczc.nl`
- Each region has its own DNS record pointing to its local gateway


#### RegionBound Mode

Use **RegionBound Mode** when your workload is deployed in only one region and cross-region latency is acceptable for users in other regions.

![RegionBound-mode](../../assets/images/docs/networking/Mode-Regionbound.png)

- WEU ExternalDNS creates record in WEU DNS zone
- NEU ExternalDNS creates record in NEU DNS zone
- Both records point to WEU cluster
- Clients in both regions route to WEU cluster, regardless of client location

#### Comparison

| Aspect | Active Mode | RegionBound Mode |
|--------|-------------|------------------|
| **DNS Scope** | Each cluster manages only its region | One cluster manages multiple regions |
| **Controller Selection** | Only matching cluster's region | All configured regions |
| **Traffic Pattern** | Regional (clients route locally) | Centralized (clients route cross-region) |
| **Use Case** | Latency optimization, data sovereignty | Cost optimization, regions without clusters |
| **Failover** | Regional (per-cluster) | Centralized (single source cluster) |
| **Policy Activation** | Active in all clusters (by default) | Active only in `sourceRegion` cluster |

### DNS Resolution Flow

Regardless of operational mode, the DNS resolution flow is the same:

```
1. Client Query
   → myapp-ns-p-prod-app1.aks.vecp.vczc.nl

2. DNS Server: CNAME Lookup
   → Returns: aks01-weu-ingressgateway-internal.aks.vecp.vczc.nl

3. DNS Server: A Record Lookup
   → Returns: 10.123.45.67 (Load Balancer IP)

4. Client Connects
   → 10.123.45.67:443 (Istio Ingress Gateway)

5. Istio Routes
   → Backend service in User Subnet
```




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
