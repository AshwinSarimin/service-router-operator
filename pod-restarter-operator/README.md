## Step 1: Scaffold the Project

```bash
cd /mnt/c/git/service-router-operator/
mkdir pod-restarter-operator
cd pod-restarter-operator

# Initialize the Go module
echo "Initializing Kubebuilder project..."
kubebuilder init \
  --domain platform.com \
  --repo github.com/AshwinSarimin/service-router-operator/pod-restarter \
  --project-name pod-restarter-operator
```

## Create a API

```bash
kubebuilder create api \
  --group chaos \
  --version v1alpha1 \
  --kind PodRestarter \
  --resource \
  --controller
```

## Step 4: Verify Initial Build
make manifests

# Generate code
make generate

# Build binary
make build
If successful, you'll see:
go build -o bin\manager.exe cmd/main.go
The binary is now in bin\manager.exe (but it doesn't do anything useful yet).

## Step 2: Define the API Types

pod-restarter-operator\api\v1alpha1\podrestarter_types.go

- Simple spec with selector and interval
- Status with restart counts and timestamps
- Validation markers

## Step 3: Implement Controller Logic

pod-restarter-operator\internal\controller\podrestarter_controller.go

- Find pods matching selector
- Check if interval has elapsed
- Delete pod (Kubernetes recreates it)
- Update status

## Update Sample CR

pod-restarter-operator\config\samples\chaos_v1alpha1_podrestarter.yaml


## Step 5: Regenerate Manifests and Build

```bash
# Generate CRD and RBAC manifests
make manifests

# Generate DeepCopy code
make generate

# Format code
make fmt

# Check for issues
make vet

# Build
make build
```

## Check the generated CRD:

cat config\crd\bases\chaos.platform.com_podrestarters.yaml

## Step 4: Test Locally

Let's test the operator logic without a cluster.
make test

Part 4: Testing in Kind

# Create cluster
kind create cluster --name pod-restarter-test

# Verify
kubectl cluster-info --context kind-pod-restarter-test
kubectl get nodes

Step 2: Install CRDs

# Install the PodRestarter CRD
make install

# Verify
kubectl get crd podrestarters.chaos.platform.com
kubectl describe crd podrestarters.chaos.platform.com

Step 3: Run Operator Locally
Open a new terminal and run:

```bash
cd C:\GIT\service-router-operator\pod-restarter
make run
```

Expected output:
2025-01-07T10:30:00Z    INFO    setup   starting manager
2025-01-07T10:30:01Z    INFO    Starting EventSource    {"controller": "podrestarter"}
2025-01-07T10:30:01Z    INFO    Starting Controller     {"controller": "podrestarter"}
2025-01-07T10:30:01Z    INFO    Starting workers        {"controller": "podrestarter", "worker count": 1}

Leave this terminal running - it's your log viewer!

Step 4: Deploy Demo Application
Open a third terminal:

# Deploy demo app with 3 replicas
kubectl create deployment demo-app --image=nginx:alpine --replicas=3

# Wait for pods
kubectl get pods -l app=demo-app --watch
Press Ctrl+C when all 3 pods are running.

Step 5: Create PodRestarter
# Apply the sample
kubectl apply -f config/samples/chaos_v1alpha1_podrestarter.yaml

Step 6: Watch the Magic!

Terminal 2 (Operator Logs):
2025-01-07T10:32:00Z    INFO    First restart - will restart pods immediately
2025-01-07T10:32:00Z    INFO    Restarting pod (rolling strategy)   {"pod": "demo-app-xxx-yyy"}
2025-01-07T10:32:00Z    INFO    Successfully restarted pods     {"count": 1, "strategy": "rolling"}

Terminal 3 (Watch Pods):
kubectl get pods -l app=demo-app --watch
You'll see pods terminating and recreating!

Check Status:
kubectl get podrestarter

Detailed Status:
kubectl describe podrestarter podrestarter-sample

Step 7: Test Different Strategies

All strategy (restart all at once):
```yaml
kubectl apply -f - <<EOF
apiVersion: chaos.platform.com/v1alpha1
kind: PodRestarter
metadata:
  name: restarter-all
spec:
  selector:
    matchLabels:
      app: demo
  intervalMinutes: 2
  strategy: all
EOF
```

Random-one strategy (one random pod):
```yaml
kubectl apply -f - <<EOF
apiVersion: chaos.platform.com/v1alpha1
kind: PodRestarter
metadata:
  name: restarter-random
spec:
  selector:
    matchLabels:
      app: demo
  intervalMinutes: 2
  strategy: random-one
EOF
```

Step 8: Test Suspend
# Suspend restarts
```bash
kubectl patch podrestarter podrestarter-sample -p '{"spec":{"suspend":true}}' --type=merge
```

# Resume
```bash
kubectl patch podrestarter podrestarter-sample -p '{"spec":{"suspend":false}}' --type=merge
```

Cleanup Kind
```bash
kind delete cluster --name pod-restarter-test
```

Part 5: Testing in Homelab
Step 1: Build Container Image
# Build image (replace with your registry)
```bash
make docker-build IMG=teknologieur1acr.azurecr.io/pod-restarter:v0.1.0
```

# Push to registry
```bash
az login --tenant 99f9af3a-fd8b-41ec-b487-483a0c562b5c
az acr login --name teknologieur1acr
make docker-push IMG=teknologieur1acr.azurecr.io/pod-restarter:v0.1.0
```

Step 2: Deploy to Homelab
# Switch to homelab context
```bash
kubectl config get-contexts
kubectl config use-context kubernetes-admin@kubernetes
```

# Install CRDs
```bash
make install
```

# Deploy operator
```bash
make deploy IMG=teknologieur1acr.azurecr.io/pod-restarter:v0.1.0

# Verify
kubectl get pods -n pod-restarter-operator-system
kubectl logs -n pod-restarter-operator-system deployment/pod-restarter-operator-controller-manager -f
```

Step 3: Create Test Workload
```bash
# Create namespace
kubectl create namespace test-chaos

# Deploy test app
kubectl create deployment test-app --image=nginx:alpine --replicas=5 -n test-chaos
```

Step 4: Create PodRestarter
```bash
kubectl apply -n test-chaos -f - <<EOF
apiVersion: chaos.platform.com/v1alpha1
kind: PodRestarter
metadata:
  name: test-restarter
spec:
  selector:
    matchLabels:
      app: test-app
  intervalMinutes: 5
  strategy: rolling
  maxConcurrent: 1
EOF
```

Step 5: Monitor
```bash
# Watch pods
kubectl get pods -n test-chaos --watch

# Check status
kubectl get podrestarter -n test-chaos test-restarter -o yaml

# Check logs
kubectl logs -n pod-restarter-operator-system deployment/pod-restarter-operator-controller-manager -f
```

Cleanup Homelab
```bash
kubectl delete namespace test-chaos
make undeploy
make uninstall
```

# Quick Reference

## Common Commands

```bash
# Build
make manifests generate build

# Test
make test

# Run locally
make run

# Deploy
make docker-build docker-push IMG=<image>
make deploy IMG=<image>

# Cleanup
make undeploy
make uninstall
```

#################################################################

# Troubleshooting
Pods not restarting?
- Check operator logs
- Verify label selector matches
- Check if suspended

Operator not starting?

- Check CRDs installed: kubectl get crd
- Check RBAC permissions
- Check operator logs

Build fails?

- Run make manifests generate
- Check Go syntax
- Verify imports

What You Learned
Kubernetes

Custom Resource Definitions (CRDs)
Controllers and reconciliation
Label selectors
Pod lifecycle
RBAC permissions

Go

Struct tags
Pointers vs values
Error handling
Context usage
Time operations
Method receivers
Multiple returns

Operator Patterns

Time-based reconciliation
Status management
Requeue strategies
Condition management


Next Steps

Add unit tests for restart logic
Add metrics (Prometheus)
Add webhooks for validation
Add time windows (business hours only)
Write blog posts about your journey