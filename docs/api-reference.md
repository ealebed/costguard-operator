# API Reference

## Group/Version/Kind

- Group: `finops.ealebed.github.io`
- Version: `v1alpha1`
- Kind: `BudgetNamespace`

## Spec

### `namespaceName` (string, required)

Target namespace managed by this resource.

### `labels` / `annotations` (map[string]string, optional)

Merged onto target namespace metadata.

### `quota` (required)

`ResourceQuota` hard limits:

- `cpu`
- `memory`
- `storage`
- `persistentVolumeClaims`
- `pods`

### `defaults` (required)

`LimitRange` default request/limit values:

- `requestCPU`
- `requestMemory`
- `limitCPU`
- `limitMemory`

### `ttl` (duration string, optional)

Namespace lifecycle TTL, e.g. `72h`.

### `enforcement` (optional)

- `enabled` (default true)
- `action` (`ScaleToZero` or `None`, default `ScaleToZero`)
- `restoreOnRecovery` (default false)
- `enforcementCooldown` (Go duration, default `2m`)

### `costBudget` (optional)

BigQuery-backed spend enforcement:

- `enabled`
- `billingExportTable` (`project.dataset.table`)
- `clusterName`
- `billingLocation` (e.g. `EU`, `US`)
- `maxSpendUSD` (decimal string)
- `lookbackWindow` (duration, default `168h`)
- `queryInterval` (duration, default `1h`)

## Status

- `observedGeneration`
- `managedNamespace`
- `expiresAt`
- `lastEnforcementAt`
- `lastEnforcementOperation` (`ScaleToZero` / `Restore`)
- `lastCostQueryAt`
- `lastObservedSpendUSD`
- `conditions[]`

Common condition types:

- `Ready`
- `Expired`
- `OverBudget`
- `EnforcementRecoveryDeferred`

## Sample (cost budget)

```yaml
apiVersion: finops.ealebed.github.io/v1alpha1
kind: BudgetNamespace
metadata:
  name: budgetnamespace-costalloc
spec:
  namespaceName: costalloc-test-ns
  quota:
    cpu: "2"
    memory: "4Gi"
    storage: "20Gi"
    persistentVolumeClaims: 5
    pods: 10
  defaults:
    requestCPU: "100m"
    requestMemory: "128Mi"
    limitCPU: "500m"
    limitMemory: "512Mi"
  ttl: 720h
  enforcement:
    enabled: true
    action: ScaleToZero
    restoreOnRecovery: true
    enforcementCooldown: 2m
  costBudget:
    enabled: true
    billingExportTable: ylebi-rnd.gcp_billing_export.gcp_billing_export_resource_v1_011BB4_25F476_819B61
    clusterName: costguard-gke
    billingLocation: EU
    maxSpendUSD: "0.03"
    lookbackWindow: 24h
    queryInterval: 15m
```
