# Development

This document provides steps and guidance for the Service Router operator development.

## Development Workflow

```bash
# Install CRDs
make install

# Run tests
make test

# Run operator locally against configured cluster
make run

# Generate code after API changes
make generate

# Update CRD manifests after API changes
make manifests

# Sync CRDs to Helm chart
make sync-crds
```

## Testing

```bash
# Unit tests
make test

# Integration tests
cd test/integration && ginkgo -v .

# Controller tests
cd internal/controller/routing && ginkgo -v .
```

## Helm Chart Development

The chart's `crds/` directory is generated from `config/crd/bases/`. Before installing or packaging:

```bash
# Sync CRDs to chart
make sync-crds

# Install chart locally
helm install service-router-operator charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace
```

## Contributing

### Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Run `gofmt` before committing
- Use meaningful variable names
- Add comments for exported functions
- Follow [Logging Standards](LOGGING-STANDARDS.md) for controller log messages

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add RegionBound mode support
fix: correct DNS hostname construction
docs: update architecture diagram
test: add ServiceRoute controller tests
```

### Pull Request Process

1. Create feature branch from `main`
2. Make changes with tests
3. Run `make test` and `make manifests`
4. Submit PR with description
5. Address review comments

## Architecture Overview

### Directory Structure

- `api/`: CRD type definitions organized into two API groups:
  - `api/cluster/v1alpha1/`: ClusterIdentity, DNSConfiguration (cluster-scoped)
  - `api/routing/v1alpha1/`: Gateway, DNSPolicy, ServiceRoute (namespace-scoped)
- `internal/controller/`: Six controller implementations:
  - `internal/controller/cluster/`: ClusterIdentity, DNSConfiguration controllers
  - `internal/controller/routing/`: Gateway, DNSPolicy, IngressDNS, ServiceRoute controllers
- `internal/clusteridentity/`: Shared cache and utilities for cluster metadata
- `internal/dnsconfiguration/`: DNS controller registry cache
- `config/`: Kubernetes manifests for deployment
- `charts/`: Helm chart with auto-synced CRDs

### CRD Architecture

The operator defines **5 CRDs across 2 API groups**:

**cluster.router.io/v1alpha1** (cluster-scoped):
- ClusterIdentity: Cluster metadata (region, domain, environment)
- DNSConfiguration: ExternalDNS controller registry

**routing.router.io/v1alpha1** (namespace-scoped):
- Gateway: Istio Gateway wrapper with DNS target configuration
- DNSPolicy: DNS propagation strategy per namespace
- ServiceRoute: Per-service DNS and routing configuration

### What the Operator Manages

The operator automatically creates and manages:
- **DNSEndpoint CRDs**: CNAME and A records for ExternalDNS
- **Istio Gateway resources**: With aggregated hostnames from ServiceRoutes

The operator does **NOT** create:
- **Istio VirtualService resources**: Users must create these to route traffic

### Adding a New CRD

1. Run Kubebuilder scaffolding:

```bash
   # For cluster-scoped resources
   kubebuilder create api --group cluster --version v1alpha1 --kind NewResource --namespaced=false
   
   # For namespace-scoped resources
   kubebuilder create api --group routing --version v1alpha1 --kind NewResource
```

2. Define types in `api/{group}/v1alpha1/newresource_types.go`

3. Generate code and manifests:

```bash
   make generate
   make manifests
```

4. Implement controller in `internal/controller/{group}/`

5. Add RBAC markers:

```go
   //+kubebuilder:rbac:groups=routing.router.io,resources=newresources,verbs=get;list;watch;create;update;patch;delete
   //+kubebuilder:rbac:groups=routing.router.io,resources=newresources/status,verbs=get;update;patch
   //+kubebuilder:rbac:groups=routing.router.io,resources=newresources/finalizers,verbs=update
```

6. Register controller in `cmd/main.go`

7. Sync CRDs to Helm chart:

```bash
   make sync-crds
```
