# Cluster Configuration

This directory contains the cluster-level Flux Kustomization resources.

## Files

- **configmaps.yaml**: Contains substitution variables for infrastructure components
- **infrastructure.yaml**: Defines Flux Kustomization resources that deploy infrastructure controllers
- **kustomization.yaml**: Main kustomization that includes all resources

## Setup

Before deploying, update the values in `configmaps.yaml`:

### ExternalDNS Configuration

```yaml
DNS_REGION: "weu"              # Region: weu, neu, etc.
ENV_LETTER: "p"                # Environment: p (prod), d (dev), t (test)
CLUSTER: "aks01"               # Cluster identifier
TENANT: "myorg"                # Your organization/tenant name
CHART_VERSION: "1.14.x"        # ExternalDNS Helm chart version
IMAGE_TAG: "v0.14.0"           # ExternalDNS image tag
```

### Azure Workload Identity

Replace the placeholder UUIDs with actual values from your Azure environment:

```bash
# Get the managed identity client ID
az identity show \
  --resource-group <RESOURCE_GROUP> \
  --name id-external-dns-weu \
  --query clientId -o tsv

# Get your tenant ID
az account show --query tenantId -o tsv

# Get your subscription ID
az account show --query id -o tsv
```

Update these in `configmaps.yaml`:
- `MANAGED_IDENTITY_CLIENT_ID`: Client ID of the managed identity for ExternalDNS
- `TENANT_ID`: Azure AD tenant ID
- `GLOBAL_SUBSCRIPTION_ID`: Azure subscription ID
- `GLOBAL_PRIVATEDNS_RG`: Resource group containing the Private DNS zones

## Deployment

These resources are automatically deployed by Flux when synced with the Git repository:

1. ConfigMaps are created in the `flux-system` namespace
2. Kustomization resources use `postBuild.substituteFrom` to inject the ConfigMap values
3. The infrastructure controllers are deployed with the substituted values

## Multi-Region Setup

To add a second region (e.g., North Europe):

1. Uncomment the `external-dns-neu` ConfigMap in `configmaps.yaml`
2. Update the values for the NEU region
3. Uncomment the corresponding Kustomization in `infrastructure.yaml`
4. Create the managed identity and federated credential for NEU
