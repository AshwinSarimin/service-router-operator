# Service Router Operator Helm Chart

This Helm chart deploys the Service Router Operator to Kubernetes clusters.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- CRD installation (handled automatically)

## Installing the Chart

### Using Helm

```bash
# Install with default values
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace

# Install with custom values
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --set image.repository=myregistry/service-router-operator \
  --set image.tag=v0.1.0
```

### Using Makefile

```bash
# Install via Makefile (uses default IMG variable)
make helm-install

# Install with custom image
IMG=myregistry/service-router-operator:v0.1.0 make helm-install
```

## Configuration

The following table lists the configurable parameters and their default values.

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of controller replicas | `1` |
| `image.repository` | Controller image repository | `controller` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Controller image tag | `""` (uses appVersion) |
| `namespace` | Namespace for deployment | `service-router-system` |

### Controller Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.logLevel` | Log level (debug, info, error) | `info` |
| `controller.leaderElection` | Enable leader election | `true` |
| `controller.development` | Enable development mode | `false` |

### Resources

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Production Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `production.enabled` | Enable production settings | `false` |
| `production.replicaCount` | Production replica count | `3` |
| `production.resources.limits.cpu` | Production CPU limit | `1000m` |
| `production.resources.limits.memory` | Production memory limit | `512Mi` |
| `production.podAntiAffinity.enabled` | Enable pod anti-affinity | `true` |

### Monitoring

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable metrics service | `true` |
| `metrics.port` | Manager metrics bind port (inside pod) | `8080` |
| `metrics.service.type` | Service type for metrics Service | `ClusterIP` |
| `metrics.service.port` | Service port for metrics (kube-rbac-proxy listens here) | `8443` |
| `metrics.service.annotations` | Annotations to add to the metrics Service | `{}` |
| `metrics.kubeRbacProxy.enabled` | Deploy kube-rbac-proxy sidecar to terminate TLS and proxy metrics | `true` |
| `metrics.kubeRbacProxy.image` | Image repository/tag/pullPolicy for kube-rbac-proxy | `gcr.io/kubebuilder/kube-rbac-proxy:v0.14.1` |
| `metrics.kubeRbacProxy.args` | Arg list passed to kube-rbac-proxy (secure-listen-address, upstream, etc.) | see `values.yaml` |
| `metrics.kubeRbacProxy.port` | Container port the kube-rbac-proxy listens on | `8443` |
| `metrics.kubeRbacProxy.resources` | Resource requests/limits for kube-rbac-proxy | `{}` |
| `serviceMonitor.enabled` | Create ServiceMonitor for Prometheus | `false` |
| `serviceMonitor.interval` | Scrape interval | `30s` |

### High Availability

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podDisruptionBudget.enabled` | Enable PodDisruptionBudget | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods | `1` |

### Health probes

| Parameter | Description | Default |
|-----------|-------------|---------|
| `healthProbe.port` | Manager health probe bind port (used by kubelet probes) | `8081` |
| `livenessProbe.httpGet.path` | Path used for liveness probe | `/healthz` |
| `livenessProbe.httpGet.port` | Port used for liveness probe | `8081` |
| `livenessProbe.initialDelaySeconds` | Liveness probe initial delay | `15` |
| `livenessProbe.periodSeconds` | Liveness probe period | `20` |
| `readinessProbe.httpGet.path` | Path used for readiness probe | `/readyz` |
| `readinessProbe.httpGet.port` | Port used for readiness probe | `8081` |
| `readinessProbe.initialDelaySeconds` | Readiness probe initial delay | `5` |
| `readinessProbe.periodSeconds` | Readiness probe period | `10` |

## Examples

### Development Environment

```bash
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --set controller.logLevel=debug \
  --set controller.development=true \
  --set controller.leaderElection=false \
  --set replicaCount=1
```

### Production Environment

```bash
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --set production.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set serviceMonitor.enabled=true \
  --set image.tag=v0.1.0
```

### With Custom Image Registry

```bash
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --set image.repository=myregistry.io/service-router-operator \
  --set image.tag=latest \
  --set image.pullPolicy=Always
```

## Upgrading

```bash
# Upgrade with new values
helm upgrade service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --set image.tag=v0.2.0

# Upgrade via Makefile
IMG=myregistry/service-router-operator:v0.2.0 make helm-install
```

## Uninstalling

```bash
# Uninstall via Helm
helm uninstall service-router-operator --namespace service-router-system

# Uninstall via Makefile
make helm-uninstall
```

## Values File

For complex configurations, create a `values.yaml` file:

```yaml
# values-production.yaml
production:
  enabled: true

podDisruptionBudget:
  enabled: true

serviceMonitor:
  enabled: true
  interval: 30s

image:
  repository: myregistry.io/service-router-operator
  tag: v0.1.0
  pullPolicy: Always

controller:
  logLevel: info

nodeSelector:
  workload-type: operator

tolerations:
  - key: "operator"
    operator: "Equal"
    value: "true"
    effect: "NoSchedule"
```

Then install with:

```bash
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  -f values-production.yaml
```

## Monitoring

### Prometheus Integration

When `serviceMonitor.enabled=true`, the chart creates a ServiceMonitor resource that Prometheus Operator can discover:

```bash
helm install service-router-operator ./charts/service-router-operator \
  --namespace service-router-system \
  --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.prometheus=kube-prometheus
```

### Metrics Endpoint

The controller exposes metrics on port 8080 at `/metrics`. Access via:

```bash
kubectl port-forward -n service-router-system \
  svc/service-router-operator-metrics-service 8443:8443

curl -k https://localhost:8443/metrics
```

Note: by default the chart deploys a `kube-rbac-proxy` sidecar that terminates TLS on port 8443 and proxies to the manager's pod-local metrics endpoint at `127.0.0.1:8080`.
This is controlled by the following values in `values.yaml`:

- `metrics.kubeRbacProxy.enabled` (bool) — enable/disable the proxy (default: `true`).
- `metrics.kubeRbacProxy.image` — repository/tag/pullPolicy for the sidecar image.
- `metrics.kubeRbacProxy.args` — list of args passed to the proxy (secure-listen-address, upstream, etc.).
- `metrics.kubeRbacProxy.port` — container port the proxy listens on (default: `8443`).

If you disable the proxy (`metrics.kubeRbacProxy.enabled=false`) the manager will bind the metrics endpoint on a network-accessible address (`:8080`) and you should update the Service/ServiceMonitor to scrape the manager directly (or provide an alternate Service targeting the manager port). Keeping the proxy enabled is recommended for production since it enforces RBAC and serves metrics over TLS.

## CRDs

This chart includes the operator's CRD manifests under `crds/` so installing the chart will create the CRDs before other resources. The CRDs are copied from `config/crd/bases/` and are authoritative YAML files (not templated).

Files included:

- `crds/cluster.router.io_clusterregions.yaml`
- `crds/routing.router.io_serviceroutes.yaml`
- `crds/dns.router.io_dnspolicies.yaml`
- `crds/gateway.router.io_gateways.yaml`
- `crds/istio.networking.istio.io_gateways.yaml`

To sync the CRDs from the source directory into the chart, use the repository Makefile target:

```bash
make sync-crds
```

This will copy all YAML files from `config/crd/bases/` into `charts/service-router-operator/crds/` so they are packaged with the chart.

## Troubleshooting

### Check Controller Logs

```bash
kubectl logs -n service-router-system \
  -l control-plane=controller-manager \
  --tail=100 -f
```

### Check Health Status

```bash
kubectl get pods -n service-router-system
kubectl describe pod -n service-router-system <pod-name>
```

### Verify CRDs

```bash
kubectl get crds | grep router.io
```

### Check Leader Election

When running multiple replicas:

```bash
kubectl get lease -n service-router-system
```

## Support

For issues and questions:
- GitHub Issues: https://github.com/vecozo/service-router-operator/issues
- Documentation: https://github.com/vecozo/service-router-operator
