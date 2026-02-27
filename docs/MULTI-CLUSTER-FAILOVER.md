# Multi-Cluster and Failover Guide

## Overview

The Service Router Operator supports multi-cluster, multi-region deployments with built-in failover capabilities. This guide explains how the operator manages DNS across multiple clusters and how failover works when a cluster becomes unavailable.

## Architecture Diagrams

- **[Multi-Cluster Architecture](../diagrams/multi-cluster-architecture.png)** - Shows how multiple AKS clusters in different regions operate independently
- **[Failover Scenario](../diagrams/failover-scenario.png)** - Demonstrates the failover process when a cluster fails

## Multi-Cluster Architecture

### Key Concepts

1. **Each Region Has Its Own AKS Cluster**
   - West Europe (WEU): `aks-weu-01`
   - North Europe (NEU): `aks-neu-01`
   - France Central (FRC): `aks-frc-01`

2. **Each Cluster Runs Its Own Instances**
   - Service Router Operator (with all controllers)
   - **Multiple ExternalDNS instances** - one for each region's DNS zone
     - Example: WEU cluster runs `external-dns_weu` and `external-dns_neu`
     - Each instance is pre-configured to write to its corresponding regional DNS zone
     - This enables seamless failover without ExternalDNS reconfiguration
   - Application workloads

3. **Shared Azure Private DNS Zones**
   - Each region has its own Private DNS Zone (e.g., `example-weu.private`, `example-neu.private`)
   - Multiple ExternalDNS instances can write to the same zone
   - DNS zones are shared infrastructure, not cluster-specific

### How It Works

#### Normal Operation (Active Mode)

In **Active Mode**, each cluster manages DNS records for its own region:

```yaml
# In WEU Cluster
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: weu
  cluster: aks01
  domain: example.com
  environmentLetter: p
---
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: default
  namespace: myapp
spec:
  mode: Active  # Regional isolation
  sourceRegion: weu
```

**What Happens:**
1. WEU cluster ServiceRoute controller generates a `DNSEndpoint` with target `aks01-weu-internal.example.com`
2. `external-dns-weu` (running in WEU cluster) picks up the DNSEndpoint
3. `external-dns-weu` writes DNS record to **WEU Private DNS Zone**:
   - `myapp-p-web.example.com` → `aks01-weu-internal.example.com` → `20.50.100.10`
4. WEU users query WEU DNS zone and get WEU cluster IP
5. Traffic stays within the region

**Same process happens independently in NEU and FRC clusters for their respective regions.**

#### Cross-Region Mode (RegionBound Mode)

In **RegionBound Mode**, a single cluster manages DNS for all regions:

```yaml
apiVersion: routing.router.io/v1alpha1
kind: DNSPolicy
metadata:
  name: default
  namespace: myapp
spec:
  mode: RegionBound  # Cross-region consolidation
  sourceRegion: weu
  sourceCluster: aks01
```

**What Happens:**
1. WEU cluster generates DNSEndpoints for **ALL regions** (WEU, NEU, FRC)
2. Three ExternalDNS instances in WEU cluster write to three DNS zones:
   - `external-dns-weu` writes to WEU DNS zone
   - `external-dns-neu` writes to NEU DNS zone
   - `external-dns-frc` writes to FRC DNS zone
3. All DNS records point to the WEU cluster: `20.50.100.10`
4. Users in all regions get the WEU cluster IP
5. Cross-region traffic flows to WEU

**Use Cases:**
- Cost optimization (run fewer clusters)
- Centralized traffic management
- During migrations

## Failover Mechanism

### The Problem: Cluster Failure

When a cluster fails:
1. ❌ The Service Router Operator stops running
2. ❌ ExternalDNS stops running
3. ❌ DNS records become stale (no updates)
4. ❌ After TTL expires, DNS queries may fail or get stale IPs
5. ❌ Users in that region cannot access services

### The Solution: `adoptsRegions` 

The Service Router Operator supports **DNS takeover** through the `adoptsRegions` field in ClusterIdentity.

#### Step-by-Step Failover Process

**Scenario:** WEU cluster (`aks-weu-01`) fails

**Step 1: Platform Team Action**

Update the ClusterIdentity in a healthy cluster (e.g., NEU) to adopt the failed region:

```yaml
# In NEU Cluster
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: neu
  cluster: aks01
  domain: example.com
  environmentLetter: p
  adoptsRegions:  # ⭐ KEY FIELD
    - weu         # NEU cluster adopts WEU region
```

**Step 2: Operator Reconciliation**

The NEU cluster's Service Router Operator detects the change:

1. Reads `adoptsRegions: [weu]`
2. For applications with `DNSPolicy mode: Active`, it now includes **both** regions:
   - Own region: `neu`
   - Adopted region: `weu`
3. ServiceRoute controller generates **two** DNSEndpoints:
   - One for NEU zone: `myapp-p-web.example.com` → `aks01-neu-internal.example.com`
   - One for WEU zone: `myapp-p-web.example.com` → `aks01-neu-internal.example.com` ⭐

**Step 3: DNS Takeover**

ExternalDNS instances in the healthy NEU cluster take over DNS management for the failed region.

**Key Architecture Detail:** Each cluster runs **multiple ExternalDNS instances** - one for each region:

```yaml
# In NEU Cluster (normal operation and during failover)
- external-dns-neu  # Owner ID: external-dns-neu
  Watches: DNSEndpoints with controller annotation = external-dns-neu
  Writes to: NEU Private DNS Zone

- external-dns-weu  # Owner ID: external-dns-weu
  Watches: DNSEndpoints with controller annotation = external-dns-weu
  Writes to: WEU Private DNS Zone
```

**This is why failover is seamless:** The `external-dns-weu` instance in the NEU cluster was **already running** and configured to write to the WEU DNS zone. When the NEU operator generates WEU DNSEndpoints (due to `adoptsRegions`), this instance immediately picks them up and updates the WEU DNS zone.

**How ExternalDNS Ownership Works:**

ExternalDNS uses **TXT records** to track ownership:

```
# Before Failover (WEU cluster running)
myapp-p-web.example.com           A      20.50.100.10
_external-dns-owner.myapp-p-web   TXT    "external-dns-weu"
```

```
# After Failover (NEU cluster's external-dns-weu instance takes over)
myapp-p-web.example.com           A      20.50.200.10  ⭐ Updated
_external-dns-owner.myapp-p-web   TXT    "external-dns-weu"  ⭐ Same owner
```

Because the **owner ID matches** (`external-dns-weu`), the ExternalDNS instance running in the NEU cluster (which has the same owner ID configuration) can take over and update the DNS record without conflict.

**Important:** Both clusters run `external-dns-weu` instances with the **same owner ID**. This is intentional design:
- WEU cluster: `external-dns-weu` instance writes to WEU DNS zone (normal operation)
- NEU cluster: `external-dns-weu` instance is ready to write to WEU DNS zone (failover ready)
- When WEU fails, NEU's `external-dns-weu` instance seamlessly takes over because the owner ID matches

**Step 4: Automatic Traffic Rerouting**

1. ✅ WEU DNS zone now points to NEU cluster IP: `20.50.200.10`
2. ✅ Users in WEU query DNS and get the NEU cluster IP
3. ✅ HTTPS requests from WEU users flow to NEU cluster
4. ✅ **No manual DNS updates required**
5. ✅ **No code changes required**
6. ✅ **No user intervention required**

### Failover Characteristics

| Aspect | Details |
|--------|---------|
| **Detection Time** | Depends on monitoring/alerting (manual trigger) |
| **Takeover Time** | ~30-60 seconds after ClusterIdentity update |
| **DNS Propagation** | Depends on TTL (typically 60-300 seconds) |
| **Traffic Impact** | Users get new IP on next DNS query |
| **Application Impact** | None (same application, different cluster) |
| **Data Consistency** | Application responsible for data replication |

### Recovery Process

When the failed cluster comes back online:

**Option 1: Remove Adoption (Return to Normal)**

```yaml
# In NEU Cluster - Remove adoption
apiVersion: cluster.router.io/v1alpha1
kind: ClusterIdentity
metadata:
  name: cluster-identity
spec:
  region: neu
  cluster: aks01
  domain: example.com
  environmentLetter: p
  adoptsRegions: []  # Remove WEU
```

1. NEU operator stops generating WEU DNSEndpoints
2. NEU's `external-dns-weu` instance stops updating WEU DNS zone
3. WEU cluster starts, its `external-dns-weu` takes over again
4. DNS records switch back to WEU cluster IP

**Option 2: Permanent Migration**

Keep the adoption in place and decommission the old cluster.

## Multi-Cluster Deployment Patterns

### Pattern 1: Active-Active (Regional Isolation)

**Use Case:** Data residency, regional compliance, low latency

**Configuration:**
- Each cluster uses `DNSPolicy mode: Active`
- Each cluster manages its own region's DNS
- No `adoptsRegions` configured

**Traffic Flow:**
- WEU users → WEU DNS → WEU cluster
- NEU users → NEU DNS → NEU cluster
- FRC users → FRC DNS → FRC cluster

**Pros:**
- True regional isolation
- No cross-region traffic
- Each region independent

**Cons:**
- Higher infrastructure cost (multiple clusters)
- Data replication complexity

### Pattern 2: Active-Passive (Standby Clusters)

**Use Case:** Disaster recovery, cost optimization

**Configuration:**
- Primary cluster uses `DNSPolicy mode: Active`
- Standby clusters deployed but not serving traffic
- Failover via `adoptsRegions`

**Traffic Flow (Normal):**
- All users → Primary cluster's region DNS → Primary cluster

**Traffic Flow (Failover):**
- Standby cluster adopts primary region
- All users → Standby cluster

**Pros:**
- Lower cost (standby clusters can be smaller)
- Fast failover
- Simpler data replication (one active database)

**Cons:**
- Cross-region traffic during failover
- Higher latency for some users during failover

### Pattern 3: Hub-Spoke (Centralized)

**Use Case:** Centralized management, cost optimization

**Configuration:**
- Hub cluster uses `DNSPolicy mode: RegionBound`
- Spoke clusters minimal or not deployed

**Traffic Flow:**
- All users → All regional DNS zones → Hub cluster

**Pros:**
- Lowest infrastructure cost
- Centralized management
- Simplified operations

**Cons:**
- Cross-region traffic always
- Higher latency
- Hub cluster is single point of failure (unless using adoptsRegions for failover)

## Best Practices

### DNS Zone Design

1. **Use Separate Zones Per Region**
   ```
   example-weu.private
   example-neu.private
   example-frc.private
   ```
   
2. **Set Appropriate TTLs**
   - Production: 60-300 seconds (balance between cache and failover speed)
   - Development: 30-60 seconds (faster testing)

3. **Monitor DNS Propagation**
   - Use Azure DNS Analytics
   - Alert on stale records

### ExternalDNS Configuration

**Critical Architecture:** Each cluster must run **multiple ExternalDNS instances** - one for each region's DNS zone.

**Example for 2-region setup (WEU, NEU):**

1. **In WEU Cluster - Deploy Two ExternalDNS Instances**

   ```yaml
   # external-dns-weu deployment (manages WEU DNS zone)
   args:
     - --txt-owner-id=external-dns-weu
     - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-weu
     - --azure-resource-group=rg-dns-weu
     - --domain-filter=example-weu.private
   
   # external-dns-neu deployment (ready for NEU DNS zone takeover)
   args:
     - --txt-owner-id=external-dns-neu
     - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-neu
     - --azure-resource-group=rg-dns-neu
     - --domain-filter=example-neu.private
   ```

2. **In NEU Cluster - Deploy Two ExternalDNS Instances**

   ```yaml
   # external-dns-neu deployment (manages NEU DNS zone)
   args:
     - --txt-owner-id=external-dns-neu
     - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-neu
     - --azure-resource-group=rg-dns-neu
     - --domain-filter=example-neu.private
   
   # external-dns-weu deployment (ready for WEU DNS zone takeover)
   args:
     - --txt-owner-id=external-dns-weu
     - --annotation-filter=external-dns.alpha.kubernetes.io/controller=external-dns-weu
     - --azure-resource-group=rg-dns-weu
     - --domain-filter=example-weu.private
   ```

3. **Common Configuration for All Instances**
   ```yaml
   args:
     - --registry=txt
     - --txt-prefix=_external-dns-owner.
     - --policy=sync
     - --interval=1m
   ```

**Why This Matters:**
- Each ExternalDNS instance in a cluster is **pre-configured** to write to a specific DNS zone
- During normal operation, only the instance matching the cluster's region is actively used
- During failover (with `adoptsRegions`), the instance for the failed region becomes active
- **No ExternalDNS reconfiguration needed** - just new DNSEndpoints from the operator

### Failover Automation

**Manual Trigger (Recommended for Production):**
1. Monitor cluster health (Azure Monitor, Prometheus)
2. Alert on cluster failures
3. Platform team updates ClusterIdentity `adoptsRegions`
4. Verify DNS propagation
5. Monitor traffic shift

**Automated Trigger (Advanced):**
1. Deploy automation controller in management cluster
2. Watch cluster health metrics
3. Automatically update ClusterIdentity on failure
4. Send notifications
5. Require manual approval for recovery

### Testing Failover

**Pre-Production Testing:**

```bash
# 1. Deploy test application in WEU cluster
kubectl apply -f test-app.yaml

# 2. Verify DNS record
nslookup testapp-p-web.example-weu.private

# 3. Simulate failure: Update NEU ClusterIdentity
kubectl apply -f clusteridentity-adopt-weu.yaml

# 4. Verify DNS takeover
nslookup testapp-p-web.example-weu.private
# Should now return NEU cluster IP

# 5. Verify traffic flows to NEU
curl https://testapp-p-web.example-weu.private

# 6. Cleanup: Remove adoption
kubectl apply -f clusteridentity-original.yaml
```

**Production Runbook:**
1. Document failover procedures
2. Define RTO (Recovery Time Objective)
3. Define RPO (Recovery Point Objective)
4. Practice failover drills quarterly
5. Maintain on-call runbook

## Monitoring and Observability

### Key Metrics to Monitor

1. **Cluster Health**
   - Kubernetes API availability
   - Node readiness
   - Pod crash loops

2. **ExternalDNS Health**
   - DNS record sync lag
   - ExternalDNS pod status
   - DNS API errors

3. **DNS Query Metrics**
   - Query volume per zone
   - Query latency
   - NXDOMAIN rates

4. **Application Metrics**
   - Request latency by region
   - Error rates
   - Traffic distribution

### Alerts

```yaml
# Example Prometheus alerts
groups:
  - name: service-router-failover
    rules:
      - alert: ClusterDown
        expr: up{job="kubernetes-apiserver"} == 0
        for: 5m
        annotations:
          summary: "Cluster {{ $labels.cluster }} is down"
          
      - alert: ExternalDNSNotRunning
        expr: kube_deployment_status_replicas_available{deployment="external-dns"} < 1
        for: 2m
        annotations:
          summary: "ExternalDNS {{ $labels.deployment }} not running"
          
      - alert: DNSRecordStale
        expr: externaldns_registry_endpoints_total == 0
        for: 5m
        annotations:
          summary: "DNS records may be stale"
```

## Troubleshooting

### DNS Records Not Updating After Failover

**Check:**
1. ClusterIdentity has correct `adoptsRegions`
2. DNSPolicy `status.activeControllers` includes adopted region controller
3. ServiceRoute generates DNSEndpoint for adopted region
4. ExternalDNS pod for adopted region is running
5. ExternalDNS has permissions to Azure DNS

```bash
# Verify DNSPolicy status
kubectl get dnspolicy -n myapp -o yaml

# Check DNSEndpoints
kubectl get dnsendpoint -n myapp

# Check ExternalDNS logs
kubectl logs -n external-dns deployment/external-dns-weu
```

### Traffic Not Routing to New Cluster

**Check:**
1. DNS record has updated IP
2. DNS TTL has expired
3. Client has flushed DNS cache
4. Load balancer is healthy
5. Istio Gateway is configured

```bash
# Verify DNS record
nslookup myapp-p-web.example.com

# Check from client
curl -v https://myapp-p-web.example.com
```

### Conflicting DNS Updates

**Symptoms:** DNS records flapping between clusters

**Cause:** Two clusters with same ExternalDNS owner ID

**Fix:**
1. Ensure unique owner IDs
2. Use controller annotations
3. Remove old cluster's DNSEndpoints

```bash
# Find conflicting DNSEndpoints
kubectl get dnsendpoint --all-namespaces -o yaml | grep owner-id
```

## Limitations

1. **Manual Failover Trigger**: Requires platform team to update ClusterIdentity
2. **DNS Propagation Delay**: TTL-dependent (typically 60-300 seconds)
3. **Data Consistency**: Application responsible for data replication
4. **Cross-Region Latency**: Failover introduces cross-region traffic
5. **StatefulSets**: Stateful workloads require additional planning

## Future Enhancements

- Automated failover based on cluster health metrics
- DNS-based load balancing across regions
- Integration with Azure Traffic Manager
- Support for weighted traffic distribution
- Automated rollback on failed failover

## References

- [Architecture Diagram](../diagrams/servicerouter-architecture.png)
- [Multi-Cluster Architecture](../diagrams/multi-cluster-architecture.png)
- [Failover Scenario](../diagrams/failover-scenario.png)
- [ExternalDNS Documentation](https://github.com/kubernetes-sigs/external-dns)
- [Azure Private DNS Zones](https://learn.microsoft.com/en-us/azure/dns/private-dns-overview)
