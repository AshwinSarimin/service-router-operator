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

## Step 2: Define the API Types

Simple spec with selector and interval
Status with restart counts and timestamps
Validation markers

## Step 3: Implement Controller Logic

Find pods matching selector
Check if interval has elapsed
Delete pod (Kubernetes recreates it)
Update status

## Step 4: Test Locally

Create demo deployment
Apply PodRestarter CR
Watch pods restart

## Key Learning Points
