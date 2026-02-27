# Installation

## AKS Cluster

**Platform users**

1. Push the [Helm chart](../charts/service-router-operator/) to ACR.
2. Build and push the operator image to ACR.
3. Install the [CRDs](../config/crd/bases/) on the cluster with Flux (5 CRDs total).
4. Create a [Cluster Identity CR](#configure-the-cluster-identity) on the cluster with Flux.
5. Create a [DNSConfiguration](#configure-dns-controllers) on the cluster with Flux.
6. Create a [Gateway](#create-a-gateway) on the cluster with Flux.

**Workload users**

1. Create a [namespace scoped DNS policy](#create-namespace-dns-policy) for a workload namespace with Flux.
2. Create a [service route](#create-a-serviceroute) for a workload with Flux.
3. Create an Istio VirtualService to route traffic (operator does NOT create this).

## Local

### Prerequisites

- Go 1.24+ (as specified in go.mod)
- kubectl configured for your cluster
- Kubernetes cluster with:
  - Istio installed (operator creates Gateway resources; users create VirtualService resources)
  - ExternalDNS deployed (for DNS record management)

**ExternalDNS**

The operator creates `DNSEndpoint` resources that ExternalDNS watches. Ensure ExternalDNS is configured:

```yaml
# ExternalDNS deployment configuration
args:
  - --source=crd
  - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
  - --crd-source-kind=DNSEndpoint
  - --txt-owner-id=external-dns-weu  # Must match region-based pattern
  - --provider=azure-private-dns
  # Optional: Filter by controller annotation
  - --label-filter=router.io/region=weu
```

**Important**: The `--txt-owner-id` must match the pattern `external-dns-{region}` for cross-cluster DNS takeover to work correctly.

### Installation

1. **Install CRDs**:

```bash
make install
```

2. **Run locally** (for development):

```bash
make run
```

3. **Deploy to cluster**:

```bash
# Build and push image
make docker-build docker-push IMG=<your-registry>/service-router-operator:tag

# Deploy
make deploy IMG=<your-registry>/service-router-operator:tag
```


## General Usage

### **Configure the cluster identity**:

```bash
kubectl apply -f - <<EOF
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu
  cluster: aks01
  domain: example.com
  environmentLetter: p
EOF
```

### **Configure DNS controllers**:

```bash
kubectl apply -f - <<EOF
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
EOF
```

### **Create a Gateway**:

```bash
kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: Gateway
metadata:
  name: default-gateway
  namespace: istio-system
spec:
  controller: aks-istio-ingressgateway-internal
  credentialName: wildcard-tls-cert
  targetPostfix: gateway
EOF
```

### **Create namespace DNS policy**:

```bash
kubectl create namespace myapp

kubectl apply -f - <<EOF
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: myapp-dns
  namespace: myapp
spec:
  mode: Active
  # Controllers from DNSConfiguration matching cluster region
EOF
```

### **Create a ServiceRoute**:

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

5. **Create a VirtualService** (route traffic to your service):

**Important**: The operator adds your hostname to the Gateway but does NOT create VirtualService resources. You must create these:

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

6. **Verify resources created**:

```bash
# Check ServiceRoute status
kubectl get serviceroute -n myapp -o yaml

# Check DNSEndpoint (for ExternalDNS)
kubectl get dnsendpoint -n myapp

# Check Istio Gateway has your hostname
kubectl get gateway.networking.istio.io -n istio-system -o yaml | grep -A 5 hosts

# Verify VirtualService (you created this)
kubectl get virtualservice -n myapp api-route
```
