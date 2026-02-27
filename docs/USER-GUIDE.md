# User Guide

This guide is for application teams who want to expose their services using the Service Router Operator.

## Table of Contents

- [Quick Start](#quick-start)
- [Prerequisites](#prerequisites)
- [Understanding the Resources](#understanding-the-resources)
- [Creating Your First ServiceRoute](#creating-your-first-serviceroute)
- [Real-World Examples](#real-world-examples)
- [Understanding Status](#understanding-status)
- [Testing and Verification](#testing-and-verification)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Quick Start

**Goal**: Expose a service called `api` with DNS name `api-ns-p-prod-myapp.example.com`

### Step 1: Verify Prerequisites

```bash
# Check ClusterIdentity exists
kubectl get clusteridentity
# Expected: cluster-identity

# Check DNSConfiguration exists (platform team creates this)
kubectl get dnsconfiguration
# Expected: dns-config

# Check available Gateways
kubectl get gateways.routing.router.io -A
# Expected: At least one Gateway in istio-system
```

### Step 2: Create DNSPolicy

```bash
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
# Controllers are defined in cluster-scoped DNSConfiguration
# This policy will use controllers matching the cluster's region
EOF
```

### Step 3: Create ServiceRoute

```bash
kubectl apply -f - <<EOF
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
EOF
```

### Step 4: Create VirtualService

**Important**: You must create an Istio VirtualService to route traffic:

```bash
kubectl apply -f - <<EOF
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
EOF
```

### Step 5: Verify

```bash
# Check ServiceRoute status
kubectl get serviceroute -n myapp api-route

# Check DNS records created
kubectl get dnsendpoints -n myapp

# Test DNS resolution
nslookup api-ns-p-prod-myapp.example.com

# Test HTTP request
curl https://api-ns-p-prod-myapp.example.com/health
```

🎉 Done! Your service is now accessible via DNS and traffic is routed correctly.

## Prerequisites

### What You Need

Before creating ServiceRoutes, ensure:

1. **Namespace**: You have a namespace for your application
2. **Service**: Your application has a Kubernetes Service
3. **DNSConfiguration**: Platform team has created cluster-wide DNSConfiguration
4. **Gateway Access**: Read access to Gateway resources
5. **RBAC**: Permissions to create DNSPolicy and ServiceRoute in your namespace

### RBAC Permissions

Your team needs this Role in your namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: service-router-user
  namespace: myapp
rules:
  # Manage DNS policies and service routes
  - apiGroups: ["routing.router.io"]
    resources: ["dnspolicies", "serviceroutes"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  
  # Read status
  - apiGroups: ["routing.router.io"]
    resources: ["*/status"]
    verbs: ["get", "list", "watch"]
  
  # Read gateways (to reference them)
  - apiGroups: ["routing.router.io"]
    resources: ["gateways"]
    verbs: ["get", "list", "watch"]
```

### What Gets Created

When you create a ServiceRoute, the operator automatically creates:

1. **DNSEndpoint CRDs**: Tell ExternalDNS to create DNS records
2. **Istio Gateway Host Entries**: Operator adds your hostname to the Gateway's host list
3. **DNS Records**: Created by ExternalDNS (CNAME + TXT records)

**Important**: The operator does NOT create Istio VirtualService resources. You must create VirtualServices yourself to route traffic from the Gateway to your service.

## Understanding the Resources

### ClusterIdentity (Read-Only)

Managed by: **Platform Team**

Defines cluster metadata used in DNS construction:

```yaml
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu              # Region code (weu, neu, etc.)
  cluster: aks01           # Cluster identifier
  domain: example.com      # Base DNS domain
  environmentLetter: p     # Environment (p=prod, d=dev, t=test)
```

**You don't create this** - just reference values in your DNS names.

### Gateway (Read-Only, Shared)

Managed by: **Platform Team**

Defines reusable Istio Gateway infrastructure:

```yaml
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

**You reference this** in your ServiceRoute but don't create it.

**To see available Gateways**:

```bash
kubectl get gateways.routing.router.io -A
```

### DNSConfiguration (Read-Only, Cluster-Wide)

Managed by: **Platform Team**

Defines all ExternalDNS controllers available in the infrastructure:

```yaml
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
```

**You don't create this** - platform team creates it during cluster setup. Your DNSPolicy references controllers from this configuration.

### DNSPolicy (You Create, Per Namespace)

Defines how DNS records are managed for your namespace:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp  # Your namespace
spec:
  mode: Active  # or RegionBound
  # Controllers come from DNSConfiguration
  # In Active mode: only controllers matching cluster region are used
  # In RegionBound mode: all controllers are used (if sourceRegion matches)
status:
  active: true
  activeControllers:  # Populated by operator from DNSConfiguration
    - external-dns-weu
```

**One per namespace** (typically) - all ServiceRoutes in the namespace use it.

### ServiceRoute (You Create, Per Service)

Links your service to a Gateway and DNS:

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

**One per service** you want to expose.

## Creating Your First ServiceRoute

### Step 1: Create Your Namespace

```bash
kubectl create namespace myapp
```

### Step 2: Deploy Your Application

You need a Kubernetes Service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: myapp
spec:
  selector:
    app: api
  ports:
    - port: 80
      targetPort: 8080
```

### Step 3: Choose Available Gateway

```bash
# List available Gateways
kubectl get gateways.routing.router.io -A

# Example output:
# NAMESPACE       NAME               CONTROLLER                          POSTFIX
# istio-system    default-gateway    aks-istio-ingressgateway-internal   internal
# istio-system    external-gateway   aks-istio-ingressgateway-external   external
```

Choose based on your needs:

- **internal**: For services within VNet
- **external**: For public-facing services

### Step 4: Create DNSPolicy

**For regional service** (Active mode):

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
  # Controllers from DNSConfiguration matching cluster region will be used
```

**For centralized service** (RegionBound mode):

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: RegionBound
  sourceRegion: weu  # Only active in WEU cluster
  # All controllers from DNSConfiguration will be used (if active)
```

**Apply**:

```bash
kubectl apply -f dnspolicy.yaml
```

### Step 5: Create ServiceRoute

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

**Apply**:

```bash
kubectl apply -f serviceroute.yaml
```

### Step 6: Create VirtualService

**Critical**: The operator adds your hostname to the Gateway, but you must create a VirtualService to route traffic:

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

**Apply**:

```bash
kubectl apply -f virtualservice.yaml
```

### Step 7: Verify

```bash
# Check ServiceRoute created
kubectl get serviceroute -n myapp api-route

# Check status
kubectl get serviceroute -n myapp api-route -o yaml | yq '.status'

# Check DNSEndpoints created
kubectl get dnsendpoints -n myapp

# Check DNS resolution (after ~1 minute)
nslookup api-ns-p-prod-myapp.example.com

# Test HTTP request (confirms VirtualService is working)
curl https://api-ns-p-prod-myapp.example.com/health
```

### Understanding DNS Names

Your service DNS name is constructed as:

**Format**: `{serviceName}-ns-{envLetter}-{environment}-{application}.{domain}`

**Example**:

- `serviceName`: `api`
- `envLetter`: `p` (from ClusterIdentity)
- `environment`: `prod`
- `application`: `myapp`
- `domain`: `example.com` (from ClusterIdentity)
- **Result**: `api-ns-p-prod-myapp.example.com`

## Real-World Examples

**Note**: The examples below show DNSPolicy and ServiceRoute configurations. Remember to also create an Istio VirtualService for each ServiceRoute to route traffic (see [Quick Start](#quick-start) for VirtualService example).

### Example 1: Simple API Service

**Scenario**: Expose an API service regionally

```yaml
---
# DNSPolicy (one per namespace)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: api-dns
  namespace: api
spec:
  mode: Active
  # Controllers from DNSConfiguration matching cluster region
---
# ServiceRoute
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: api
spec:
  serviceName: api
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: api
```

**Result**:

- DNS: `api-ns-p-prod-api.example.com`
- Traffic: Regional (WEU clients → WEU cluster)

### Example 2: Internal Admin Service

**Scenario**: Admin panel accessible only within VNet

```yaml
---
# DNSPolicy
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: admin-dns
  namespace: admin
spec:
  mode: Active
---
# ServiceRoute (using internal gateway)
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: admin-route
  namespace: admin
spec:
  serviceName: admin
  gatewayName: internal-gateway  # Internal gateway
  gatewayNamespace: istio-system
  environment: prod
  application: admin
```

**Result**:

- DNS: `admin-ns-p-prod-admin.example.com`
- Access: Only from VNet (internal gateway)

### Example 3: Multi-Region Centralized Service

**Scenario**: Database admin tool in WEU, accessible from all regions

```yaml
---
# DNSPolicy (only active in WEU)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: dbadmin-dns
  namespace: dbadmin
spec:
  mode: RegionBound
  sourceRegion: weu
  # All controllers from DNSConfiguration will be used when active
---
# ServiceRoute
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: dbadmin-route
  namespace: dbadmin
spec:
  serviceName: dbadmin
  gatewayName: internal-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: dbadmin
```

**Result**:

- DNS in WEU: `dbadmin-ns-p-prod-dbadmin.example.com` → WEU cluster
- DNS in NEU: `dbadmin-ns-p-prod-dbadmin.example.com` → WEU cluster
- DNS in FRC: `dbadmin-ns-p-prod-dbadmin.example.com` → WEU cluster
- All regions route to WEU cluster (cross-region traffic)

### Example 4: Multiple Services, Same Gateway

**Scenario**: Microservices sharing gateway

```yaml
---
# DNSPolicy (shared by all services in namespace)
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: identity-dns
  namespace: identity
spec:
  mode: Active
---
# ServiceRoute for auth service
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: auth-route
  namespace: identity
spec:
  serviceName: auth
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: identity
---
# ServiceRoute for pep service
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: pep-route
  namespace: identity
spec:
  serviceName: pep
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: identity
---
# ServiceRoute for resource-server
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: resource-server-route
  namespace: identity
spec:
  serviceName: resource-server
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: prod
  application: identity
```

**Result**:

- DNS: `auth-ns-p-prod-identity.example.com`
- DNS: `pep-ns-p-prod-identity.example.com`
- DNS: `resource-server-ns-p-prod-identity.example.com`
- All use same Gateway, DNSPolicy

### Example 5: Development Environment

**Scenario**: Development services with shorter DNS names

```yaml
---
# DNSPolicy
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dev-dns
  namespace: myapp-dev
spec:
  mode: Active
---
# ServiceRoute
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: myapp-dev
spec:
  serviceName: api
  gatewayName: default-gateway
  gatewayNamespace: istio-system
  environment: dev
  application: myapp
```

**Result**:

- DNS: `api-ns-d-dev-myapp.example.com`
- Notice: `d` (dev environment letter) in DNS name

## Understanding Status

### ServiceRoute Status

Check if ServiceRoute is ready:

```bash
kubectl get serviceroute -n myapp api-route -o yaml | yq '.status'
```

**Example Healthy Status**:

```yaml
status:
  dnsEndpoint: api-route-external-dns-weu
  conditions:
    - type: Ready
      status: "True"
      reason: ReconciliationSucceeded
      message: ServiceRoute is active
      lastTransitionTime: "2024-12-16T10:00:00Z"
```

**Example Unhealthy Status**:

```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      reason: DNSPolicyNotFound
      message: Waiting for DNSPolicy to be configured in namespace
      lastTransitionTime: "2024-12-16T10:00:00Z"
```

### DNSPolicy Status

Check if DNSPolicy is active:

```bash
kubectl get dnspolicy -n myapp -o yaml | yq '.status'
```

**Example Active Status**:

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

**Example Inactive Status** (RegionBound mode, wrong region):

```yaml
status:
  active: false
  activeControllers: []
  conditions:
    - type: Ready
      status: "False"
      reason: PolicyInactive
      message: sourceRegion 'weu' does not match cluster region 'neu'
```

### Common Status Conditions

| Condition | Status | Reason | Meaning |
|-----------|--------|--------|---------|
| Ready | True | ReconciliationSucceeded | Everything working |
| Ready | False | DNSPolicyNotFound | DNSPolicy missing in namespace |
| Ready | False | DNSPolicyInactive | DNSPolicy exists but not active (check sourceRegion) |
| Ready | False | GatewayNotFound | Referenced Gateway doesn't exist |
| Ready | False | ClusterIdentityNotAvailable | ClusterIdentity not configured on cluster |
| Ready | False | ValidationFailed | Invalid spec (check message) |

## Testing and Verification

### Step 1: Check Resources Created

```bash
# Check ServiceRoute
kubectl get serviceroute -n myapp

# Check DNSEndpoints generated
kubectl get dnsendpoints -n myapp

# Check Istio Gateway (operator adds your hostname here)
kubectl get gateway.networking.istio.io -n istio-system -o yaml | grep -A 5 hosts
```

### Step 2: Verify DNS Records

**Using kubectl** (check DNSEndpoint):

```bash
kubectl get dnsendpoint -n myapp api-route-external-dns-weu -o yaml
```

Look for:

```yaml
spec:
  endpoints:
    - dnsName: api-ns-p-prod-myapp.example.com
      recordType: CNAME
      targets:
        - aks01-weu-internal.example.com
```

**Using Azure CLI** (check actual DNS):

```bash
az network private-dns record-set cname show \
  -g dns-rg \
  -z example.com \
  -n api-ns-p-prod-myapp
```

### Step 3: Test DNS Resolution

**From your local machine** (if VPN connected):

```bash
nslookup api-ns-p-prod-myapp.example.com
```

Expected:

```
Server:  10.0.0.4
Address: 10.0.0.4#53

api-ns-p-prod-myapp.example.com  canonical name = aks01-weu-internal.example.com.
Name:   aks01-weu-internal.example.com
Address: 10.123.45.67
```

**From within cluster**:

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup api-ns-p-prod-myapp.example.com
```

### Step 4: Test HTTP Request

```bash
# From local machine (if VPN connected)
curl https://api-ns-p-prod-myapp.example.com/health

# From within cluster
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl https://api-ns-p-prod-myapp.example.com/health
```

### Step 5: Check Traffic Flow

**Verify Istio Gateway has your hostname**:

```bash
# Check Istio Gateway (operator adds your hostname)
kubectl get gateway.networking.istio.io -n istio-system default-gateway -o yaml | grep -A 10 hosts

# Should show your hostname in the list:
spec:
  servers:
    - hosts:
        - api-ns-p-prod-myapp.example.com
        # ... other hostnames
```

**Create VirtualService (you must create this)**:

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

## Troubleshooting

### ServiceRoute Shows "Not Ready"

**Check status reason**:

```bash
kubectl get serviceroute -n myapp api-route \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

**Common reasons and fixes**:

#### 1. DNSPolicyNotFound

```
message: Waiting for DNSPolicy to be configured in namespace
```

**Fix**: Create DNSPolicy in the same namespace:

```bash
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
EOF
```

#### 2. DNSPolicyInactive

```
message: DNSPolicy is not active for this cluster (check sourceRegion/sourceCluster)
```

**Fix**: Check DNSPolicy status:

```bash
kubectl get dnspolicy -n myapp -o yaml | yq '.status'
```

If `active: false`, check:

- In Active mode: Should always be active
- In RegionBound mode: `sourceRegion` must match cluster region

**Solution**: Update DNSPolicy `sourceRegion` or switch to Active mode.

#### 3. GatewayNotFound

```
message: Gateway default-gateway not found
```

**Fix**: Check Gateway exists:

```bash
kubectl get gateways.routing.router.io -n istio-system
```

If missing, ask platform team to create Gateway or reference correct name.

#### 4. ClusterIdentityNotAvailable

```
message: Waiting for ClusterIdentity to be configured
```

**Fix**: Contact platform team - ClusterIdentity is managed by them.

### DNS Not Resolving

**Symptoms**: `nslookup` fails or returns no results

**Debugging**:

```bash
# 1. Check DNSEndpoints created
kubectl get dnsendpoints -n myapp

# 2. Check DNSEndpoint spec
kubectl get dnsendpoint -n myapp api-route-external-dns-weu -o yaml

# 3. Check ExternalDNS logs
kubectl logs -n external-dns -l app=external-dns-weu --tail=50

# 4. Check Azure DNS records
az network private-dns record-set cname show \
  -g dns-rg \
  -z example.com \
  -n api-ns-p-prod-myapp
```

**Common causes**:

1. **ExternalDNS not running**: Check with platform team
2. **DNS propagation delay**: Wait 1-2 minutes after creation
3. **Wrong DNS zone**: Check ClusterIdentity domain
4. **Ownership conflict**: Another controller owns the record

### Traffic Not Reaching Service

**Symptoms**: DNS resolves but HTTP request fails

**Debugging**:

```bash
# 1. Check service exists
kubectl get svc -n myapp api

# 2. Check pods are ready
kubectl get pods -n myapp -l app=api

# 3. Check VirtualService exists (you must create this)
kubectl get virtualservice -n myapp api-route -o yaml

# 4. Check Istio Gateway has your hostname
kubectl get gateway.networking.istio.io -n istio-system default-gateway -o yaml | grep -A 5 hosts

# 5. Test directly to service (bypass Istio)
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl http://api.myapp.svc.cluster.local/health
```

**Common causes**:

1. **Service not ready**: Check pod logs and status
2. **Missing VirtualService**: You must create VirtualService to route traffic (operator does NOT create this)
3. **Wrong service name**: ServiceRoute `serviceName` must match Service name
4. **VirtualService misconfiguration**: Check host and gateway references
5. **TLS certificate issue**: Check Gateway `credentialName` secret exists
6. **Firewall/Network Policy**: Check network policies in namespace

### Multiple DNSEndpoints Created

**Symptoms**: More DNSEndpoints than expected

**This is normal** if:

- Using RegionBound mode
- DNSPolicy has multiple `externalDNSControllers`
- One DNSEndpoint is created per active controller

**Example**:

```bash
kubectl get dnsendpoints -n myapp
# NAME                              AGE
# api-route-external-dns-weu        5m
# api-route-external-dns-neu        5m
```

This means records are being created in both WEU and NEU DNS zones.

### Unexpected DNS Target

**Symptoms**: CNAME points to wrong cluster

**Check DNSPolicy mode**:

```bash
kubectl get dnspolicy -n myapp -o yaml | yq '.spec.mode'
```

- **Active**: Each cluster creates records pointing to itself
- **RegionBound**: Only source cluster creates records, all point to it

**Check which cluster is active**:

```bash
kubectl get dnspolicy -n myapp -o yaml | yq '.status.active'
# If false, this cluster is not managing DNS
```

## Best Practices

### Naming Conventions

**ServiceRoute Names**: Use descriptive names indicating the service

```yaml
# Good
name: api-route
name: auth-route
name: database-proxy-route

# Avoid
name: route-1
name: service
name: myroute
```

**DNSPolicy Names**: One per namespace, use namespace-related name

```yaml
# Good
name: myapp-dns
name: identity-dns
name: api-dns

# Avoid
name: dns-policy
name: policy-1
```

### Label Standards

Add labels for better tracking:

```yaml
metadata:
  labels:
    app: myapp
    team: platform
    environment: production
```

### When to Use Active vs RegionBound

**Use Active Mode** when:

- ✅ Service runs in multiple regions
- ✅ Want regional traffic routing (latency optimization)
- ✅ Data sovereignty requirements
- ✅ Each region independent

**Use RegionBound Mode** when:

- ✅ Service only runs in one region
- ✅ Want to serve traffic from regions without clusters
- ✅ Cost optimization (fewer clusters)
- ✅ Centralized service (database, admin tools)

### Resource Organization

**One DNSPolicy per Namespace**:

```
namespace: myapp
  ├── DNSPolicy: myapp-dns (one for all services)
  ├── ServiceRoute: api-route
  ├── ServiceRoute: auth-route
  └── ServiceRoute: web-route
```

**Separate Namespaces by Team/Application**:

```
namespace: team-a-api
  ├── DNSPolicy: team-a-api-dns
  └── ServiceRoute: api-route

namespace: team-b-frontend
  ├── DNSPolicy: team-b-frontend-dns
  └── ServiceRoute: web-route
```

### Gateway Selection

**Internal Gateway**: For services within VNet

- Admin panels
- Internal APIs
- Database frontends

**External Gateway**: For public-facing services

- Customer-facing APIs
- Web applications
- Public documentation

### Documentation

Document your ServiceRoutes:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: ServiceRoute
metadata:
  name: api-route
  namespace: myapp
  annotations:
    description: "Main API endpoint for MyApp"
    owner: "team-a@company.com"
    docs: "https://wiki.company.com/myapp/api"
spec:
  # ...
```

### Testing Before Production

Test in development environment first:

```bash
# 1. Create in dev namespace
kubectl apply -f serviceroute.yaml -n myapp-dev

# 2. Verify DNS and traffic
nslookup api-ns-d-dev-myapp.example.com
curl https://api-ns-d-dev-myapp.example.com/health

# 3. Deploy to production
kubectl apply -f serviceroute.yaml -n myapp-prod
```

### Monitoring

Monitor your ServiceRoutes:

```bash
# Check all ServiceRoutes status
kubectl get serviceroutes -A \
  -o custom-columns=\
NAME:.metadata.name,\
NAMESPACE:.metadata.namespace,\
READY:.status.conditions[?(@.type=="Ready")].status

# Set up alerts for NotReady status
# (work with platform team)
```

## Next Steps

### Learn More

- **[Architecture](ARCHITECTURE.md)**: Understand how the system works
- **[ExternalDNS Integration](EXTERNALDNS-INTEGRATION.md)**: Deep dive into DNS provisioning
- **[Migration Guide](MIGRATION.md)**: Migrate from Helm chart (if applicable)

### Advanced Topics

- Multi-region consolidation strategies
- Custom DNS patterns
- Integration with CI/CD pipelines
- Automated testing of DNS records

### Get Help

- **Platform Team**: Contact for ClusterIdentity, Gateway, and ExternalDNS issues
- **Documentation**: Start with [Troubleshooting](#troubleshooting) section
- **GitHub Issues**: Report bugs or request features
