# User Guide

This guide is for application teams who want to expose services using the Service Router Operator.

## Quick Start

**Goal**: Expose a service called `api` with DNS name `api-ns-p-prod-myapp.example.com`

### Prerequisites

```bash
# Verify platform resources exist (created by platform team)
kubectl get clusteridentity
kubectl get dnsconfiguration
kubectl get gateways.routing.router.io -A
```

### Step 1: Create DNSPolicy

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active   # or RegionBound — see Modes section below
```

### Step 2: Create ServiceRoute

```yaml
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
```

**DNS name constructed**: `api-ns-p-prod-myapp.example.com`

Format: `{serviceName}-ns-{envLetter}-{environment}-{application}.{domain}`

### Step 3: Create VirtualService

The operator adds your hostname to the Istio Gateway but does **not** create a VirtualService. You must create it:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: api-route
  namespace: myapp
spec:
  hosts:
    - api-ns-p-prod-myapp.example.com
  gateways:
    - istio-system/default-gateway
  http:
    - route:
        - destination:
            host: api.myapp.svc.cluster.local
            port:
              number: 80
```

### Step 4: Verify

```bash
kubectl get serviceroute -n myapp api-route
kubectl get dnsendpoints -n myapp
nslookup api-ns-p-prod-myapp.example.com
```

---

## Understanding the Resources

### Resources managed by platform team (read-only for you)

**ClusterIdentity** — defines region, cluster name, domain, and environment letter used in DNS name construction.

**DNSConfiguration** — lists all ExternalDNS controllers available across the platform.

**Gateway** — wraps the Istio ingress gateway. Reference by name in your ServiceRoute.

```bash
# See available Gateways
kubectl get gateways.routing.router.io -A
```

### Resources you manage

**DNSPolicy** (one per namespace): defines how DNS is propagated for your services.

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active        # Active or RegionBound
  # sourceRegion: weu   # Required only for RegionBound mode
```

**ServiceRoute** (one per service): links a service to a Gateway and triggers DNS record creation.

```yaml
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
```

---

## Choosing a DNS Mode

**Active Mode** — each cluster manages DNS for its own region. Use when:
- Service runs in multiple regions
- You want regional traffic routing (low latency)
- Data residency requirements

**RegionBound Mode** — one cluster manages DNS for all regions. Use when:
- Service only runs in one region
- You want to serve users in other regions from a single cluster
- Cost optimization (fewer cluster deployments)

```yaml
# RegionBound example — only active in WEU cluster
spec:
  mode: RegionBound
  sourceRegion: weu
```

---

## Status Reference

```bash
# Check ServiceRoute status
kubectl get serviceroute -n myapp api-route -o yaml | grep -A 15 status

# Check DNSPolicy status
kubectl get dnspolicy -n myapp -o yaml | grep -A 10 status
```

| Condition Reason | Meaning | Fix |
|---|---|---|
| `ReconciliationSucceeded` | Everything working | — |
| `DNSPolicyNotFound` | No DNSPolicy in namespace | Create a DNSPolicy |
| `DNSPolicyInactive` | RegionBound mode, wrong region | Check `sourceRegion` matches cluster |
| `GatewayNotFound` | Referenced Gateway missing | Check gateway name and namespace |
| `ClusterIdentityNotAvailable` | Platform config missing | Contact platform team |

---

## Troubleshooting

### ServiceRoute not Ready

```bash
kubectl get serviceroute -n myapp api-route \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

Check the `reason` field against the Status Reference table above.

### DNS not resolving

```bash
# 1. Confirm DNSEndpoints were created
kubectl get dnsendpoints -n myapp

# 2. Check ExternalDNS logs
kubectl logs -n external-dns -l app=external-dns-weu --tail=50

# 3. Check Azure DNS records
az network private-dns record-set cname show \
  -g dns-rg -z example.com -n api-ns-p-prod-myapp
```

Common causes: ExternalDNS not running, DNS propagation delay (~1 min), wrong DNS zone in ClusterIdentity.

### Traffic not reaching service

```bash
# 1. Confirm VirtualService exists (you must create this)
kubectl get virtualservice -n myapp api-route

# 2. Confirm Gateway has your hostname
kubectl get gateway.networking.istio.io -n istio-system default-gateway \
  -o yaml | grep -A 5 hosts

# 3. Test service directly (bypass Istio)
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl http://api.myapp.svc.cluster.local/health
```

---

## Next Steps

- [Architecture](ARCHITECTURE.md) — understand how the system works end-to-end
- [ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md) — deep dive into DNS provisioning and ownership
