# ExternalDNS Integration

The Service Router Operator does **not** create DNS records directly. It creates `DNSEndpoint` custom resources that ExternalDNS watches to provision records in Azure Private DNS (or other providers).

| Component | Responsibility |
|---|---|
| Service Router Operator | Creates DNSEndpoint CRDs with hostnames, targets, and labels |
| ExternalDNS | Watches DNSEndpoints and creates/updates/deletes actual DNS records |

---

## Required ExternalDNS Configuration

Each region requires its own ExternalDNS deployment. The key parameters:

```yaml
args:
  # Source: watch DNSEndpoint CRDs only
  - --source=crd
  - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
  - --crd-source-kind=DNSEndpoint
  - "--managed-record-types=CNAME"
  - "--managed-record-types=A"
  - "--managed-record-types=TXT"

  # Filtering: only process DNSEndpoints for this region
  - --label-filter=router.io/region=weu

  # Ownership: MUST follow pattern external-dns-{region}
  - --txt-owner-id=external-dns-weu
  - --txt-prefix=weu-p-aks-          # unique per cluster

  # Provider
  - --provider=azure-private-dns
  - --azure-resource-group=dns-rg
  - --azure-subscription-id=<subscription-id>
  - --domain-filter=aks.example.com

  # Policy: upsert-only is safer for production
  - --policy=upsert-only
  - --interval=1m
```

**Critical**: `--txt-owner-id` must follow the pattern `external-dns-{region}`. This is what enables cross-cluster DNS takeover within the same region (see [Cross-Cluster Takeover](#cross-cluster-dns-takeover)).

**Label filter**: The operator sets a `router.io/region` label on every DNSEndpoint to control which ExternalDNS instance processes it. This prevents WEU ExternalDNS from creating records in the NEU DNS zone, and vice versa.

**Policy**:
- `upsert-only` — creates and updates records, never deletes. Safer for production; stale records remain after ServiceRoute deletion.
- `sync` — creates, updates, and deletes. Cleans up stale records but can cause accidental deletion if DNSEndpoints are removed unexpectedly.

---

## DNSEndpoint Structure

This is what the operator creates per ServiceRoute per active controller:

```yaml
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: api-route-external-dns-weu
  namespace: myapp
  labels:
    router.io/region: weu                    # matches ExternalDNS --label-filter
    router.io/controller: external-dns-weu
    router.io/serviceroute: api-route
  annotations:
    external-dns.alpha.kubernetes.io/controller: external-dns-weu
  ownerReferences:
    - kind: ServiceRoute
      name: api-route
      controller: true
      blockOwnerDeletion: true              # auto-deleted when ServiceRoute is deleted
spec:
  endpoints:
    - dnsName: api-ns-p-prod-myapp.example.com
      recordType: CNAME
      targets:
        - aks-weu-internal.example.com
      recordTTL: 300
```

In **RegionBound mode**, the operator creates one DNSEndpoint per active controller. For a policy with both `external-dns-weu` and `external-dns-neu` active, you'll see two endpoints — both with the same CNAME target (the source cluster's gateway), but each routed to a different ExternalDNS instance.

---

## Owner ID and TXT Records

ExternalDNS uses TXT records to track ownership and prevent multiple instances from interfering with each other.

When ExternalDNS creates a DNS record, it also creates a TXT record:

```
Name:  weu-p-aks-api-ns-p-prod-myapp.example.com
Type:  TXT
Value: "external-dns-weu"
```

Rules:
- If no TXT record exists → ExternalDNS creates the DNS record and TXT record
- If TXT record exists with **matching** owner → ExternalDNS can update or delete the record
- If TXT record exists with **different** owner → ExternalDNS ignores the record entirely

This prevents two ExternalDNS instances (e.g., WEU and NEU) from managing the same DNS record.

---

## Cross-Cluster DNS Takeover

Within the same region, two clusters can share the same `--txt-owner-id` value. This allows one cluster to take over DNS management from the other — for example, during a failover.

**Setup**: both WEU clusters use `--txt-owner-id=external-dns-weu` but different prefixes:

```
aks-weu: --txt-owner-id=external-dns-weu  --txt-prefix=weu-p-aks-
aks02-weu: --txt-owner-id=external-dns-weu  --txt-prefix=weu-p-aks02-
```

When aks fails and aks02's operator creates a new DNSEndpoint for the same hostname, aks02's ExternalDNS sees the TXT record owner matches and updates the DNS record. No manual intervention needed.

**Note**: Takeover only works within the same region. WEU and NEU clusters have different owner IDs and cannot take over each other's records. Cross-region DNS management uses RegionBound mode instead.

---

## Gateway A Records

CNAME records created by ServiceRoutes point to a gateway hostname (e.g., `aks-weu-internal.example.com`). The operator's **IngressDNS controller** creates A records for these gateway hostnames by watching the LoadBalancer Service associated with each Gateway CRD.

```
Client query: api-ns-p-prod-myapp.example.com
  → CNAME: aks-weu-internal.example.com   (created by ServiceRoute controller)
  → A:     10.123.45.67                     (created by IngressDNS controller)
  → Connects to Istio ingress gateway
```

The two-level design means gateway IP changes only require updating one A record. All CNAME records automatically follow.

---

## Debugging

```bash
# Check DNSEndpoints exist and have correct labels
kubectl get dnsendpoints -A -o wide

# Check ExternalDNS logs
kubectl logs -n external-dns -l app=external-dns-weu --tail=50

# Check Azure DNS records
az network private-dns record-set cname list \
  -g dns-rg -z example.com

# Debug DNS resolution from within cluster
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup api-ns-p-prod-myapp.example.com
```

**DNSEndpoints exist but no DNS records**: check ExternalDNS logs for ownership conflicts or permission errors. Verify the `--label-filter` matches the label on the DNSEndpoint.

**DNS records not updating**: check if the TXT record has a different owner ID — ExternalDNS will ignore the record. Delete the old TXT record or align owner IDs.

---

## References

- [ExternalDNS CRD Source](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/contributing/crd-source.md)
- [ExternalDNS Azure Private DNS](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/azure-private-dns.md)
- [Architecture](ARCHITECTURE.md)
