# Unit Tests for multi-region-service-router Helm Chart

This directory contains unit tests for the multi-region-service-router Helm chart using the [helm-unittest](https://github.com/quintush/helm-unittest) plugin.

## Test Coverage

The tests validate:

1. **DNSEndpoint Resources**:
   - Creation of DNSEndpoints for each external DNS controller
   - Correct handling of regionbound services
   - Proper CNAME target generation based on gateway settings

2. **Istio Gateway Resources**:
   - Creation of Gateway resources with correct settings
   - Proper host assignment to gateways

3. **Combined Validation**:
   - Consistency between DNS records and Gateway hosts

4. **Error Handling**:
   - Validation that regionbound apps must specify a region
   - Validation that gateways referenced in services exist in the gateways list

## Understanding the Regionbound Services Logic

The chart implements region-specific DNS record filtering for regionbound services:

- Each external DNS controller creates its own DNSEndpoint resource
- For regionbound services, a DNS record is included in **ALL controllers** when:
  - The service's region matches the CLUSTER's region
  - This means services that match the cluster region appear in ALL controllers
  - Services that don't match the cluster region won't appear in any controller

For example, with `region: neu` in the cluster values:

- Services with `region: neu` will have DNS records in ALL controllers
- Services with `region: weu` won't have DNS records in any controller
- Both NEU and WEU controllers will be created and both will have endpoints for NEU services

## Running the Tests

To run these tests, you need to install the helm-unittest plugin:

```bash
helm plugin install https://github.com/helm-unittest/helm-unittest
```

Then, run the tests from the chart directory:

```bash
cd /path/to/charts/platform/multi-region-service-router
helm unittest .
```

If you encounter issues with the combined tests, you can run individual test files using the `-f` flag:

```bash
# Test DNSEndpoint functionality
helm unittest -f tests/dnsendpoint_test.yaml .

# Test Gateway functionality
helm unittest -f tests/istio-gateway_test.yaml .

# Test error handling
helm unittest -f tests/error_test.yaml .
```

### Troubleshooting Test Issues

If you encounter YAML parsing errors or document index issues:

1. **Use the `--strict=false` flag** to ignore some validation errors:

   ```bash
   helm unittest . --strict=false
   ```

2. **Test individual files** with the `-f` flag:

   ```bash
   helm unittest -f tests/istio-gateway_test.yaml . --strict=false
   ```

3. **Check document order and structure** by rendering the template:

   ```bash
   helm template . -f tests/values/regionbound-test.yaml > output.yaml
   ```

4. **Inspect the actual document structure** to confirm which records appear in which DNS controller:

   ```bash
   # Extract the NEU controller resources
   grep -A 20 "name: external-dns-neu" output.yaml
   
   # Extract the WEU controller resources
   grep -A 20 "name: external-dns-weu" output.yaml
   ```

5. **Remember that regionbound services** only appear when they match the cluster region:
   - When cluster region is `neu`, services with `region: neu` appear in ALL controllers
   - When cluster region is `weu`, services with `region: weu` appear in ALL controllers
   - Services with regions that don't match the cluster region won't appear in any controller

6. **For path errors with special characters**, use the bracket notation:

   ```yaml
   metadata.annotations["external-dns.alpha.kubernetes.io/controller"]
   ```

7. **Debug using the `--debug` flag** with Helm template for more information:

   ```bash
   helm template --debug -f tests/values/regionbound-test.yaml .
   ```

## Alternative Testing Approaches

If you continue to experience issues with helm-unittest, consider these alternative approaches:

### Manual Template Validation

Verify templates manually using:

```bash
# Render templates with values
helm template . -f tests/values/basic-values.yaml > output.yaml

# Review the output
cat output.yaml
```

### Integration with CI/CD

For automated validation in CI/CD pipelines:

1. Use `helm lint` to check for syntax issues
2. Use `helm template` with grep to verify expected resources
3. Consider full end-to-end tests with a tool like KinD (Kubernetes in Docker)

## Complete Debugging and Fixing Guide

### Understanding the DNSEndpoint Logic for Regionbound Services

The chart's core logic for regionbound services:

1. Each DNS controller creates its own DNSEndpoint resource
2. For regionbound services, records are created when the app's region matches the cluster region:
   - When cluster region is `neu`, services with `region: neu` appear in ALL controllers
   - When cluster region is `weu`, services with `region: weu` appear in ALL controllers

### Step-by-Step Debugging Approach

1. **Create Test Values File**:

   ```yaml
   # tests/values/regionbound-test.yaml
   cluster: aks01
   region: neu
   environmentLetter: d
   domain: aks.example.com
   externalDns:
     - controller: external-dns-neu
       region: neu
     - controller: external-dns-weu
       region: weu
   apps:
     - name: app-01
       services:
         - name: service-01
       environment: dev
       mode: regionbound
       region: neu
     - name: app-02
       services:
         - name: service-02
       environment: dev
       mode: regionbound
       region: weu
   gateways:
     - name: default-gateway-ingress
       controller: istio-ingressgateway
       credentialName: example-cert
       targetPostfix: external
   ```

2. **Render and Inspect the Template**:

   ```bash
   helm template . -f tests/values/regionbound-test.yaml > output.yaml
   cat output.yaml
   ```

3. **Analyze DNSEndpoint Resources**:

   The expected structure is:

   ```yaml
   # First document: NEU controller with NEU service
   apiVersion: externaldns.k8s.io/v1alpha1
   kind: DNSEndpoint
   metadata:
     name: external-dns-neu
     ...
   spec:
     endpoints:
     - dnsName: service-01-ns-d-dev-app-01.aks.example.com
       ...
       
   # Second document: WEU controller with WEU service
   apiVersion: externaldns.k8s.io/v1alpha1
   kind: DNSEndpoint
   metadata:
     name: external-dns-weu
     ...  
   spec:
     endpoints:
     - dnsName: service-02-ns-d-dev-app-02.aks.example.com
       ...
   ```

4. **Fix the Test Assertions**:

   ```yaml
   asserts:
     # Check NEU controller (doc 0) has NEU service
     - isKind:
         of: DNSEndpoint
       documentIndex: 0
     - equal:
         path: metadata.name
         value: external-dns-neu
       documentIndex: 0
     - matchRegex:
         path: spec.endpoints[0].dnsName
         pattern: service-01-ns-d-dev-app-01.aks.example.com
       documentIndex: 0
       
     # Check WEU controller (doc 1) has WEU service
     - isKind:
         of: DNSEndpoint
       documentIndex: 1  
     - equal:
         path: metadata.name
         value: external-dns-weu
       documentIndex: 1
     - matchRegex:
         path: spec.endpoints[0].dnsName
         pattern: service-02-ns-d-dev-app-02.aks.example.com
       documentIndex: 1
   ```

### Key Test Structure Requirements

1. **Well-Structured Release Section**:

   ```yaml
   release:
     name: test-release
     namespace: default
   ```

2. **Use Proper Path Notation**:

   Use bracket notation for annotations and other paths with special characters:

   ```yaml
   - equal:
       path: metadata.annotations["external-dns.alpha.kubernetes.io/controller"]
       value: external-dns-neu
   ```

3. **Understand Document Ordering**:

   - The template generates DNSEndpoint resources in the order they appear in `externalDns`
   - Each resource gets its own document index (0, 1, 2, etc.)
   - Services appear in endpoints only for matching regions

4. **Endpoints Array Is Zero-Indexed**:

   The first DNS record in each controller is at `spec.endpoints[0]`

5. **Use `lengthEqual` for Checking Array Lengths**:

   ```yaml
   # Check that spec.endpoints has exactly 1 item
   - lengthEqual:
       path: spec.endpoints
       count: 1
     documentIndex: 0
   ```

   Note: Don't use `hasDocuments` for checking array lengths - it's for counting YAML documents!

6. **For Combined Template Tests, Use Explicit Template Specification**:

   ```yaml
   - isKind:
       of: Gateway
     documentIndex: 0
     template: istio-gateway.yaml  # Explicitly specify which template file
   ```

   This is crucial because:
   - Document indexes reset for each template file
   - The first document in each template has documentIndex: 0
   - Without explicit template specification, assertions may target the wrong file

## Solution to the Regionbound Test Issues

The main issue with the regionbound services test case was a misunderstanding of how the template's filtering logic works:

1. **Initial Assumption**: The test was incorrectly assuming that services would only be present in controllers matching their region

2. **Actual Template Behavior**: The template filters based on whether the app's region matches the **cluster's region**:

   ```yaml
   {{- if eq $appRegion $region }}  # Compares app's region to CLUSTER region
     {{- $shouldCreateRecord = true }}
   {{- end }}
   ```

   This check is executed for **each controller** in the loop, so when it evaluates to true, the record is added to ALL controllers.

3. **Fixed Test Approach**:
   - For cluster `region: neu`:
     - Apps with `region: neu` get DNS records in ALL controllers
     - Apps with `region: weu` get NO DNS records
     - Both NEU and WEU controllers include records for NEU services
   - For cluster `region: weu`:  
     - Apps with `region: weu` get DNS records in ALL controllers
     - Apps with `region: neu` get NO DNS records
     - Both NEU and WEU controllers include records for WEU services

## Expected Output

When tests pass successfully, you should see output similar to:

```output
### Chart [ multi-region-service-router ] .

✓ dnsendpoint template tests - should create DNSEndpoint resources for each external DNS controller
✓ dnsendpoint template tests - should handle regionbound services correctly
✓ dnsendpoint template tests - should use custom gateway settings when specified
✓ istio-gateway template tests - should create Gateway resources for each gateway configuration
✓ istio-gateway template tests - should create multiple Gateway resources with correct services assigned
✓ combined templates tests - should create consistent DNS and Gateway definitions for services
✓ error handling tests - should fail when a regionbound app has no region specified

Charts:      1 passed, 0 failed, 0 errored, 1 total
Test Suites: 0 passed, 0 failed, 0 errored, 0 total
Tests:       7 passed, 0 failed, 0 errored, 7 total
Snapshot:    0 passed, 0 failed, 0 total
Time:        XX.XXms
```
