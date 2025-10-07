# Kubebuilder

## Install prerequisites

```bash
# Remove any previous Go installation
sudo rm -rf /usr/local/go
sudo rm -rf /usr/bin/go

# Download Go
wget https://go.dev/dl/go1.25.1.linux-amd64.tar.gz

# Extract and install
sudo tar -C /usr/local -xzf go1.25.1.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
export PATH=$PATH:/usr/local/go/bin

go version

# Install make
sudo apt update
sudo apt install make
make --version

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/
kubectl version --client
```

## Install Kubebuilder

Kubebuilder is the framework we'll use to create the operator.

```bash
# Download and install Kubebuilder
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/

kubebuilder version
```

## Setup the Development Environment

### Configure Go Environment:
```bash
# Add to ~/.bashrc or ~/.zshrc
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
export GO111MODULE=on
```

### Reload your shell:
```bash
source ~/.bashrc  # or source ~/.zshrc
```

## Verify Everything Works

```bash
# Check Go
go version

# Check Kubebuilder
kubebuilder version

# Check kubectl
kubectl version --client

# Check Docker
docker --version
```

## Create a Project

Create a directory, and then run the init command inside of it to initialize a new project.

```bash
cd /mnt/c/git/service-router-operator/
mkdir service-router
cd service-router

# Initialize the Go module
echo "Initializing Kubebuilder project..."
kubebuilder init \
  --domain platform.com \
  --repo github.com/AshwinSarimin/service-router-operator \
  --project-name service-router-operator
```

What This Does:
- Creates go.mod with module name
- Sets up basic project structure
- Creates Makefile with common targets
- Generates PROJECT file (tracks Kubebuilder metadata)
- Creates cmd/main.go (entry point)
- Sets up config/ directory with Kustomize configs

## Create a API

Run the following command to create a new API (group/version) as webapp/v1 and the new Kind(CRD) Guestbook on it

```bash
echo "Creating ServiceRouter API..."
kubebuilder create api \
  --group networking \
  --version v1alpha1 \
  --kind ServiceRouter \
  --resource \
  --controller
```

What This Does:
- Creates api/v1alpha1/podrestarter_types.go (CRD definition)
- Creates internal/controller/podrestarter_controller.go (reconciliation logic)
- Creates internal/controller/podrestarter_controller_test.go (test stub)
- Updates cmd/main.go to register the new types
- Creates sample CR in config/samples/

# Understanding APIs
This command’s primary aim is to produce the Custom Resource (CR) and Custom Resource Definition (CRD) for the Memcached Kind. It creates the API with the group cache.example.com and version v1alpha1, uniquely identifying the new CRD of the Memcached Kind. By leveraging the Kubebuilder tool, we can define our APIs and objects representing our solutions for these platforms.

While we’ve added only one Kind of resource in this example, we can have as many Groups and Kinds as necessary. To make it easier to understand, think of CRDs as the definition of our custom Objects, while CRs are instances of them.

chaos.platform.com/v1alpha1
└─┬─┘ └──────┬─────┘ └──┬──┘
  │          │           │
  │          │           └─ Version (v1alpha1 = experimental)
  │          └─ Domain (from kubebuilder init)
  └─ Group (logical grouping of resources)

Your full API: chaos.platform.com/v1alpha1/PodRestarter

https://book.kubebuilder.io/cronjob-tutorial/gvks 


```bash
echo "Next steps:"
echo "1. Review the generated project structure"
echo "2. Edit api/v1alpha1/servicerouter_types.go to define your CRD"
echo "3. Edit internal/controller/servicerouter_controller.go to implement logic"
echo "4. Run 'make manifests' to generate CRD manifests"
echo "5. Run 'make install' to install CRDs in your cluster"
echo "6. Run 'make run' to test the operator locally"
```

## Edit api/v1/servicerouter_types.go to define your CRD"
Create the actual Go code for your operator

## Edit internal/controller/servicerouter_controller.go"
Create the controller logic

## Test 

```
make run
```

#################################################################

# Understanding Kubebuilder: Projects, APIs, CRDs, and Controllers

## Key Takeaways
The relationship between these concepts:

Kubebuilder Project = The whole application
API (Custom Resource) = Go structs that define your resource structure
CRD = YAML manifest generated from Go structs that teaches Kubernetes about your resource
Controller = The reconciliation logic that watches resources and makes changes

## What is Kubebuilder?

**Kubebuilder** is a framework for building Kubernetes APIs (operators) using Custom Resource Definitions (CRDs). Think of it as a "scaffolding tool" that generates the boilerplate code for you, so you can focus on writing the business logic.

### Why Use Kubebuilder?

Instead of manually writing thousands of lines of Kubernetes API code, Kubebuilder:
- **Generates project structure** automatically
- **Creates API definitions** (CRDs) with proper Kubernetes conventions
- **Scaffolds controllers** with reconciliation loops
- **Generates RBAC rules** for security
- **Creates Dockerfiles** and deployment manifests
- **Handles API versioning** and conversion

## The Four Core Concepts

Let's understand each concept with your service router as an example.

---

## 1. Kubebuilder Project

A **Kubebuilder project** is the entire Go application that becomes your Kubernetes operator. It's like a web application project, but instead of serving HTTP requests, it watches Kubernetes resources and reconciles them.

### Project Structure

When you run `kubebuilder init`, it creates this structure:

```
service-router-operator/
├── api/                          # API definitions (your custom resources)
│   └── v1/
│       ├── servicerouter_types.go    # CRD struct definitions
│       └── groupversion_info.go      # API group/version info
├── internal/
│   └── controller/               # Controller logic
│       └── servicerouter_controller.go  # Reconciliation logic
├── config/                       # Kubernetes manifests
│   ├── crd/                      # Generated CRD YAML
│   ├── rbac/                     # RBAC roles and bindings
│   ├── manager/                  # Operator deployment manifest
│   └── samples/                  # Example custom resources
├── cmd/
│   └── main.go                   # Entry point of the operator
├── Dockerfile                    # Container image definition
├── Makefile                      # Build and deployment commands
└── go.mod                        # Go dependencies
```

### What the Project Does

```go
// cmd/main.go - Simplified version
func main() {
    // 1. Create a manager that connects to Kubernetes
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme: scheme,
    })
    
    // 2. Register your controller with the manager
    if err = (&controller.ServiceRouterReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        log.Error(err, "unable to create controller")
    }
    
    // 3. Start the manager (runs forever, watching for changes)
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        log.Error(err, "problem running manager")
    }
}
```

**Think of it as**: A long-running process that watches Kubernetes and responds to changes.

---

## 2. Kubebuilder API (Custom Resources)

An **API** in Kubebuilder is a new resource type you're adding to Kubernetes. Just like Kubernetes has built-in resources (Pod, Service, Deployment), you're creating your own custom resource.

### Creating an API

```bash
kubebuilder create api \
  --group networking \      # API group: networking.acme.com
  --version v1 \           # Version: v1
  --kind ServiceRouter     # Resource name: ServiceRouter
```

This generates:
1. **Go struct definitions** in `api/v1/servicerouter_types.go`
2. **CRD YAML manifests** (generated by `make manifests`)

### The API Definition (Go Structs)

```go
// This is what YOU define - the structure of your custom resource
type ServiceRouter struct {
    metav1.TypeMeta   `json:",inline"`           // Kind, APIVersion
    metav1.ObjectMeta `json:"metadata,omitempty"` // Name, Namespace, Labels, etc.
    
    Spec   ServiceRouterSpec   `json:"spec,omitempty"`   // What user wants
    Status ServiceRouterStatus `json:"status,omitempty"` // Current state
}

// What the user configures
type ServiceRouterSpec struct {
    Cluster          string                    `json:"cluster"`
    Region           string                    `json:"region"`
    EnvironmentLetter string                   `json:"environmentLetter"`
    Domain           string                    `json:"domain"`
    ExternalDNS      []ExternalDNSController   `json:"externalDns"`
    Gateways         []Gateway                 `json:"gateways"`
    Apps             []Application             `json:"apps"`
}

// What the operator reports back
type ServiceRouterStatus struct {
    Conditions          []metav1.Condition `json:"conditions,omitempty"`
    DNSEndpointsCreated int32             `json:"dnsEndpointsCreated,omitempty"`
    GatewaysCreated     int32             `json:"gatewaysCreated,omitempty"`
    LastReconciled      *metav1.Time      `json:"lastReconciled,omitempty"`
}
```

### Special Annotations (Markers)

These comments above structs are **markers** that Kubebuilder reads to generate code:

```go
// +kubebuilder:validation:Required
// This field is mandatory
Cluster string `json:"cluster"`

// +kubebuilder:validation:Pattern:="^[dtp]$"
// Must match regex pattern
EnvironmentLetter string `json:"environmentLetter"`

// +kubebuilder:default:="active"
// Default value if not specified
Mode string `json:"mode,omitempty"`

// +kubebuilder:validation:Enum=active;regionbound
// Must be one of these values
Mode string `json:"mode,omitempty"`

// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// Shows this column in `kubectl get servicerouters`
```

**Think of it as**: Defining a new type of Kubernetes resource (like defining a database schema).

---

## 3. Custom Resource Definitions (CRDs)

A **CRD** is the Kubernetes manifest that tells Kubernetes about your new resource type. It's the "schema" that Kubernetes uses to validate your custom resources.

### From Go Structs to CRD YAML

When you run `make manifests`, Kubebuilder reads your Go structs and generates CRD YAML:

```yaml
# config/crd/bases/networking.acme.com_servicerouters.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: servicerouters.networking.acme.com
spec:
  group: networking.acme.com
  names:
    kind: ServiceRouter
    listKind: ServiceRouterList
    plural: servicerouters
    singular: servicerouter
  scope: Namespaced
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            required:
            - cluster
            - region
            - environmentLetter
            - domain
            properties:
              cluster:
                type: string
              region:
                type: string
              environmentLetter:
                type: string
                pattern: "^[dtp]$"
              domain:
                type: string
              externalDns:
                type: array
                items:
                  type: object
                  properties:
                    controller:
                      type: string
                    region:
                      type: string
          status:
            type: object
            properties:
              conditions:
                type: array
              dnsEndpointsCreated:
                type: integer
              gatewaysCreated:
                type: integer
```

### Installing the CRD

```bash
# Install CRD in your cluster
make install

# Now Kubernetes knows about ServiceRouter resources
kubectl get servicerouters
# NAME                CLUSTER   REGION   AGE
# my-service-router   aks01     neu      5m
```

### Using the CRD

After installing, users can create instances of your custom resource:

```yaml
# This is what users will write
apiVersion: networking.acme.com/v1
kind: ServiceRouter
metadata:
  name: my-service-router
  namespace: istio-system
spec:
  cluster: "aks01"
  region: "neu"
  environmentLetter: "d"
  domain: "aks.vecd.vczc.nl"
  externalDns:
    - controller: external-dns-neu
      region: neu
    - controller: external-dns-weu
      region: weu
  gateways:
    - name: "default-gateway-ingress"
      controller: "aks-istio-ingressgateway-internal"
      credentialName: "cert-aks-ingress"
      targetPostfix: "external"
  apps:
    - name: "nid-02"
      environment: dev
      services:
        default-gateway-ingress:
          - "auth"
          - "pep"
```

**Think of it as**: The contract between users and your operator. It defines what fields are valid, required, and their types.

---

## 4. Controllers (The Reconciliation Logic)

A **controller** is the brain of your operator. It watches for changes to your custom resources and makes the cluster match the desired state.

### The Controller Pattern

```
┌─────────────────────────────────────────────────────────────┐
│  Kubernetes API Server                                      │
│  ┌──────────────────┐                                       │
│  │  ServiceRouter   │  User creates/updates/deletes         │
│  │  Custom Resource │                                       │
│  └──────────────────┘                                       │
└─────────────┬───────────────────────────────────────────────┘
              │
              │ Watch Events
              │ (Create, Update, Delete)
              ▼
┌─────────────────────────────────────────────────────────────┐
│  Your Operator (Controller)                                  │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Reconcile Loop                                      │   │
│  │  1. Get ServiceRouter resource                      │   │
│  │  2. Calculate desired state (DNS, Gateways)         │   │
│  │  3. Get current state from cluster                  │   │
│  │  4. Make changes to match desired state             │   │
│  │  5. Update status                                    │   │
│  │  6. Requeue if needed                                │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────┬───────────────────────────────────────────────┘
              │
              │ Creates/Updates
              ▼
┌─────────────────────────────────────────────────────────────┐
│  Kubernetes Resources                                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ DNSEndpoint  │  │ DNSEndpoint  │  │   Gateway    │      │
│  │     NEU      │  │     WEU      │  │   default    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### The Reconcile Function

This is the **most important function** in your operator:

```go
func (r *ServiceRouterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    
    // STEP 1: Fetch the ServiceRouter resource
    serviceRouter := &networkingv1.ServiceRouter{}
    err := r.Get(ctx, req.NamespacedName, serviceRouter)
    if err != nil {
        if errors.IsNotFound(err) {
            // Resource deleted - cleanup if needed
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }
    
    // STEP 2: Validate the configuration
    if err := r.validateServiceRouter(serviceRouter); err != nil {
        log.Error(err, "Validation failed")
        return ctrl.Result{}, err
    }
    
    // STEP 3: Reconcile DNSEndpoint resources
    // This creates/updates DNSEndpoint resources based on the ServiceRouter spec
    dnsCount, err := r.reconcileDNSEndpoints(ctx, serviceRouter)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // STEP 4: Reconcile Gateway resources
    // This creates/updates Istio Gateway resources
    gwCount, err := r.reconcileGateways(ctx, serviceRouter)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // STEP 5: Update the status
    serviceRouter.Status.DNSEndpointsCreated = int32(dnsCount)
    serviceRouter.Status.GatewaysCreated = int32(gwCount)
    serviceRouter.Status.LastReconciled = &metav1.Now()
    
    if err := r.Status().Update(ctx, serviceRouter); err != nil {
        return ctrl.Result{}, err
    }
    
    // STEP 6: Requeue after 10 minutes for continuous reconciliation
    return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}
```

### When Reconcile is Called

The `Reconcile` function is called when:

1. **A ServiceRouter is created**
   ```bash
   kubectl apply -f servicerouter.yaml
   # → Reconcile() is called
   ```

2. **A ServiceRouter is updated**
   ```bash
   kubectl edit servicerouter my-service-router
   # → Reconcile() is called
   ```

3. **A ServiceRouter is deleted**
   ```bash
   kubectl delete servicerouter my-service-router
   # → Reconcile() is called (resource not found)
   ```

4. **An owned resource changes** (DNSEndpoint or Gateway)
   ```bash
   kubectl delete dnsendpoint external-dns-neu
   # → Reconcile() is called (to recreate it)
   ```

5. **Periodic requeue** (every 10 minutes in our case)
   ```go
   return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
   ```

### Controller Setup

```go
func (r *ServiceRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&networkingv1.ServiceRouter{}).          // Watch ServiceRouter resources
        Owns(&externaldnsv1alpha1.DNSEndpoint{}).    // Watch DNSEndpoints we create
        Owns(&istiov1.Gateway{}).                    // Watch Gateways we create
        Complete(r)
}
```

This tells Kubernetes:
- **For**: Watch for changes to `ServiceRouter` resources
- **Owns**: Watch for changes to `DNSEndpoint` and `Gateway` resources that were created by this operator
- If any of these change → call `Reconcile()`

**Think of it as**: A continuous loop that ensures your desired state always matches reality.

---

## How They Work Together: End-to-End Flow

Let's trace what happens when you create a ServiceRouter:

### 1. User Creates Resource

```bash
kubectl apply -f servicerouter.yaml
```

```yaml
apiVersion: networking.acme.com/v1
kind: ServiceRouter
metadata:
  name: my-service-router
spec:
  cluster: "aks01"
  region: "neu"
  apps:
    - name: "nid-02"
      services:
        default-gateway-ingress: ["auth", "pep"]
```

### 2. Kubernetes Validates Against CRD

Kubernetes checks:
- ✅ Is `cluster` field present? (required)
- ✅ Is `region` field present? (required)
- ✅ Does `environmentLetter` match pattern `[dtp]`?
- ✅ Are all required fields present?

If validation passes → Resource is stored in etcd

### 3. Controller Receives Event

```go
// Kubernetes notifies the controller:
// "ServiceRouter 'my-service-router' was created"
Reconcile(ctx, Request{
    NamespacedName: types.NamespacedName{
        Name:      "my-service-router",
        Namespace: "istio-system",
    },
})
```

### 4. Controller Reconciles

```go
func (r *ServiceRouterReconciler) Reconcile(ctx, req) {
    // Get the ServiceRouter
    sr := &ServiceRouter{}
    r.Get(ctx, req.NamespacedName, sr)
    
    // Create DNSEndpoint for external-dns-neu
    dnsEndpoint := &DNSEndpoint{
        Name: "external-dns-neu",
        Spec: {
            Endpoints: [
                {
                    DNSName: "auth-ns-d-dev-nid-02.aks.vecd.vczc.nl",
                    RecordType: "CNAME",
                    Targets: ["aks01-neu-external.aks.vecd.vczc.nl"],
                },
                {
                    DNSName: "pep-ns-d-dev-nid-02.aks.vecd.vczc.nl",
                    RecordType: "CNAME",
                    Targets: ["aks01-neu-external.aks.vecd.vczc.nl"],
                },
            ],
        },
    }
    r.Create(ctx, dnsEndpoint)
    
    // Create Gateway
    gateway := &Gateway{
        Name: "default-gateway-ingress",
        Spec: {
            Hosts: [
                "auth-ns-d-dev-nid-02.aks.vecd.vczc.nl",
                "pep-ns-d-dev-nid-02.aks.vecd.vczc.nl",
            ],
        },
    }
    r.Create(ctx, gateway)
    
    // Update status
    sr.Status.DNSEndpointsCreated = 1
    sr.Status.GatewaysCreated = 1
    r.Status().Update(ctx, sr)
}
```

### 5. Result

```bash
kubectl get servicerouter
# NAME                CLUSTER   REGION   DNS ENDPOINTS   GATEWAYS   AGE
# my-service-router   aks01     neu      1               1          30s

kubectl get dnsendpoint
# NAME                  ENDPOINTS   AGE
# external-dns-neu      2           30s

kubectl get gateway
# NAME                        AGE
# default-gateway-ingress     30s
```

---

## Key Concepts Summary

| Concept | What It Is | Your Role | Example |
|---------|-----------|-----------|---------|
| **Kubebuilder Project** | The entire Go application | Configure and customize | `service-router-operator/` directory |
| **API (Custom Resource)** | New resource type definition | Define the Go structs | `ServiceRouter` struct in `servicerouter_types.go` |
| **CRD** | Kubernetes manifest describing your API | Generated automatically | `servicerouters.yaml` in `config/crd/` |
| **Controller** | The reconciliation logic | Write the business logic | `Reconcile()` function in `servicerouter_controller.go` |

---

## Common Commands

```bash
# Initialize project
kubebuilder init --domain acme.com --repo github.com/you/service-router-operator

# Create API
kubebuilder create api --group networking --version v1 --kind ServiceRouter

# Generate CRD manifests from Go structs
make manifests

# Install CRDs in cluster
make install

# Run operator locally (for development)
make run

# Build and push Docker image
make docker-build docker-push IMG=your-registry/service-router-operator:v1

# Deploy operator to cluster
make deploy IMG=your-registry/service-router-operator:v1

# Uninstall CRDs
make uninstall

# Undeploy operator
make undeploy
```

---

## Your Service Router Operator Flow

```
┌─────────────────────────────────────────────────────────────┐
│ 1. User applies ServiceRouter YAML                          │
│    (defines cluster, region, apps, gateways)                │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Kubernetes validates against CRD                         │
│    (checks required fields, types, patterns)                │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Controller receives event                                │
│    Reconcile() is called                                    │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Controller logic executes                                │
│    ├─ Validates configuration                               │
│    ├─ Creates DNSEndpoint per ExternalDNS controller        │
│    ├─ Creates Istio Gateway per gateway config              │
│    └─ Updates ServiceRouter status                          │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Resources created in cluster                             │
│    ├─ DNSEndpoint: external-dns-neu                         │
│    ├─ DNSEndpoint: external-dns-weu                         │
│    └─ Gateway: default-gateway-ingress                      │
└─────────────────────────────────────────────────────────────┘
```

---

## Questions to Test Understanding

1. **What happens if you modify `servicerouter_types.go`?**
   - You need to run `make manifests` to regenerate the CRD YAML

2. **Where does the business logic go?**
   - In the `Reconcile()` function in the controller

3. **How does Kubernetes know about ServiceRouter?**
   - You install the CRD with `make install`

4. **What triggers the Reconcile function?**
   - Create/Update/Delete of ServiceRouter, changes to owned resources, or periodic requeue

5. **Can users create ServiceRouter without the CRD installed?**
   - No, Kubernetes will reject it as an unknown resource type

---

Would you like me to explain any of these concepts in more detail, or would you like to proceed with building the actual operator?


#################################################################

## Info

Kubebuilder streamlines the development of Kubernetes controllers by providing a structured framework that integrates with controller runtime and Kubernetes API's. It abstracts much of the repetitive setup, and enabled development with efficient and maintainable extensions of Kubernetes functionality.


In this chapter, I continue exploring the powerful world of Kubernetes customization using Operators — this time taking a step further and implementing a custom Kubernetes Controller with Kubebuilder. Instead of simply reacting to changes in a single custom resource, I aim to create a more interactive, policy-enforcing Controller that actively shapes the behavior of running workloads.

Let’s say you run a multi-tenant cluster where different clients are allowed to launch workloads based on some predefined quotas. In a typical case, these quotas are enforced externally — by a CI/CD system or a platform layer. But what if you could enforce runtime quotas directly inside the cluster, at the Controller level?

As a learning path, I plan to create a custom resource (CR) and a Controller with the following responsibilities:

Monitor Pod creations in a specific namespace.
Validate each Pod’s annotation against a set of known client API keys.
Maintain and update client quotas stored in a shared ConfigMap.
Track quota usage by observing which Pods are actively running.
Periodically reduce a client’s quota based on how many Pods they have running.
Automatically stop all client Pods and block new ones once the quota is exhausted.
This is not a typical “hello world” Operator. It is a small but complete policy engine, built entirely within Kubernetes using native building blocks: Custom Resources, Controllers, and the reconciliation loop — all scaffolded and composed using Kubebuilder.

In this article, I will walk you through the design and implementation of such a Controller step by step, highlighting key patterns and architectural decisions along the way. The result will be a working controller that watches, validates, enforces, and adapts, giving you practical insight into writing custom controllers beyond basic CRUD.

To keep things local and reproducible, a lightweight Kubernetes cluster based on kind is used. It spins up Kubernetes clusters inside Docker containers and is perfect for testing Operators in isolated environments.

Let’s get started with setting up the Operator project using Kubebuilder nice scaffolding feature. First, make sure you have kubebuilder, go, and docker installed on your machine. The assumption is that Go ≥1.20 and a Linux/Mac-based development environment.

More information about Operators can be found in one of my previous article:

Kubernetes Operator. Create the one with Kubebuilder.
One of possible way to customize the Kubernetes cluster is to use Operators. They extend Kubernetes capabilities by…
fenyuk.medium.com

Create a fresh directory for your project and initialize it with kubebuilder:


This command scaffolds a new Kubernetes Operator project with Go modules and a clean directory structure. It uses the domain operator.k8s.yfenyuk.io (you can change it to whatever matches your preferred naming convention), and sets the module path for your Go project.

Next, define the first custom API type — ClientQuota. This resource will represent the client’s identity and the quota they are allowed to consume:


This command generates the boilerplate for:

The API definition (api/v1alpha1/clientquota_types.go)
The controller logic stub (controllers/clientquota_controller.go)
The CRD manifest (config/crd/bases/...)
When prompted:

Say “yes” to generating both the resource and the controller.
This gives us a ready-to-extend reconciliation loop for ClientQuota resources.
At this point, the minimal skeleton Operator is ready. Next step is to build up the behavior we described earlier — validating Pods, tracking quotas, and reacting to Pod state — all via Kubernetes-native APIs and custom logic.

In the next section, I’ll define the schema of the ClientQuota resource and explain how it maps to client identity and consumption limits.

CustomResource keeps the list of Clients who are allowed to run Pods in my Cluster and their details:


Test sample in config/samples/quota_v1alpha1_clientquota.yaml
#6: target namespace is playground;

#7: start set of Clients definitions;

#8..#10: detailed Client description with its Name, secret ApiKey, and how many minutes his Pods can run in my Cluster.

The simple idea that Client buys a certain time for his Pods and receives from me a unique ApiKey which he is obliged to set in annotation to each Pod he wants to run. It is the only possibility for Pod to be run in shared playground Kubernetes namespace. Any Pod, that has no (or invalid) ApiKey will be restricted from running. In addition, if quotaMinutest reaches zero, all Client Pod will be killed. Please remember that it is for educational purposes and basic.

Same structure should be reflected as Goland code for CRD:


Manual editing of api/v1alpha1/clientquota_types.go file is required.

Next step is to generate CustomResourceDefinition YAML file. Luckily, Kubebuilder automates this also with make manifests command and the result YAML can be found in file config/crd/bases/quota.operator.k8s.yfenyuk.io_clientquotas.yaml:


quota.operator.k8s.yfenyuk.io_clientquotas.yaml
#3: CustomResourceDefinition kind;

#11: CRD has name ClientQuota;

#23..#37: already familiar structure of CRD, array of objects, each of it has Name, ApiKey and QuotaMinutes.

From now, we have both CustomResourceDefinition and CustomResource YAMLs. so we can deploy it into Kubernetes cluster to be ready for reconciliation:


The central port of each Controller is Reconsile function. The plan is to call it every minute and delete all illegal Pods and decrease the left quota for allowed Pods.

Get Yuri Fenyuk’s stories in your inbox
Join Medium for free to get updates from this writer.

Enter your email
Subscribe
Open file internal/controller/clientquota_controller.go and extend it. Since code is pretty long and can be found in my repo on github, the plan is to start from main entry Reconsile function:


#5..#8: read CustomResource ClientQuota, which keeps details of allowed Clients;

#10..#15: create(using ClientQuota as source of data) or get existing ConfigMap, which contains the left Quota for each Client (function code is below);

#22: Pod’s annotation key where ApiKey is expected. Each Client knows only its own ApiKey and should keep it in secret;

#23: start loop over found in ‘playground’ namespace Pods;

#24..#32: If for any reason Pod has no ApiKey -> kill it;

#36..#53: If Pod’s ApiKey is not specified in active CustomResource -> kill it;

#55..#64: If no ApiKey is found in ConfigMap with left Quota (shouldn’t be the case if access to it is secured) -> again kill Pod;

#65..#70: Current Pod is from legal Client, but NO Quota left -> again kill Pod;

#72..#74: decrease left Quota on one minute;

#76..#79: store ConfigMap with up-to-date Quota in Kubernetes;

#81: Return a positive reconciliation result and ask it to be called again one minute later.

Function to receive ConfigMap:


#4: try to read ConfigMap;

#8..#24: if ConfigMap is not found, initialize it and fill from passed-in function CustomResource by borrowing each Client Name and initial Quota;

#36..#44: build Golang dictionary with Name to ClientQuota and return it back to the Reconsile function.

These two Golang functions contain the logic to maintain ClientQuota state and to keep Pods in the observed namespace in sync with current quotas. Link to the clientquota_controller.go file is here.

For the first test run, Controller can be launched outside of the Kubernetes cluster with Kubebilder’s command make run to run inside VS Code, and the console output will be something like:

Press enter or click to view image in full size

First Operator run
Reconciliation has started and QuotaMap has been created and stored in Kubernetes. As there are no Pods at all, nothing happens.

Still, the initial QuotaMap has been stored (with data borrowed from CustomResource, created before):


#4: Client with Name team-x has 120 in quota;

#5: Client with Name team-y has 60 in quota.

When first allowed Client creates a Pod:


#6: put Pod in playground namespace;

#8: set correct ApiKey teamy456, which belongs to teamy;

#12..#13: Pod sleeps for one hour as an execution simulation.

The next reconciliation cycle shows some activity:

Press enter or click to view image in full size

Second log for ‘Reconciling ClientQuota…’ is followed by log message ‘ClientSpec’ and ‘Pod API Key found in quota, checking usages…’, which means that Client is allowed to run Pod since the correct ApiKey is specified in Pod’s annotation, this Pod will not be killed, controller just decreased Quota in ConfigMap for this client, which is immediately visible in it:


#5: team-y has quota 59 left.

Sure thing, when Quota becomes 0, the reconciliation logic will kill the Pod.

The final step is to build Controller as Docker image and deploy it inside the Kubernetes Cluster.
There are a few important caveats:

as I am using kind Cluster, look at Using kind remark;
need to add line imagePullPolicy: IfNotPresent into file config/manager/manager.yaml, otherwise Kubernetes will try to find Docker image in public registry, not in kind local
while running inside Cluster, Controlled will need to get/set ConfigMap and get/delete Pods, so RBAC needs to be extended with //+kubebuilder:rbac:groups=”” …, which can be inserted directly into Golang controller file.
Deploy Controller:


which should build Docker image clientquota:latest and deploy it as Kubernetes deployment into the Cluster.

If it went well, there will be new Pod with the following, on my Cluster, logs:


Same reconciliation cycle log can be found in #14..#17.
As before, left Quota in ConfigMap continue to get down every minute, and in less than an hour, Client’s Pod will be deleted. If I run two Pods with an allowed ApiKey , the whole quota will be ‘eaten’ in 30 mins.

There is a design weakness at the moment. If redelpoy same Pod after Quota has reached zero, this Pod continues to run up to one minute, until next reconciliation cycle. I plan to tackle it with Admission Control in the next chapter.

Here is full Kubebuilder project sources.

