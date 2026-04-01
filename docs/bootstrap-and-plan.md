# costguard-operator bootstrap and implementation plan

## Naming and API domain

- Project/repo: `costguard-operator`
- CRD Kind: `BudgetNamespace`
- API group example: `finops.ealebed.github.io`

## Step 0: prerequisites

- Go 1.25.x
- kubebuilder (latest v4)
- kustomize
- kubectl
- docker
- access to a GKE cluster

## Step 1: initialize operator

From repository root (`costguard-operator`):

```bash
kubebuilder init \
  --domain ealebed.github.io \
  --repo github.com/ealebed/costguard-operator
```

Then create API and controller:

```bash
kubebuilder create api \
  --group finops \
  --version v1alpha1 \
  --kind BudgetNamespace \
  --resource \
  --controller
```

## Step 2: CRD design (v1alpha1)

Recommended `spec` fields:

- `namespaceName`: target namespace to manage
- `labels`: labels applied to namespace
- `annotations`: annotations applied to namespace
- `quota`: hard limits for cpu/memory/storage/pvc/pods
- `defaults`: default requests/limits for LimitRange
- `ttl`: duration (`72h`) or explicit expiry timestamp
- `enforcement`:
  - `enabled` (default `true` for your test setup)
  - `action` (`ScaleToZero` or `None`)
  - `gracePeriod` (optional)

Recommended `status`:

- `conditions`: `Ready`, `QuotaApplied`, `LimitRangeApplied`, `OverBudget`, `Expired`
- `observedGeneration`
- `lastReconcileTime`
- `usageSnapshot` (phase 2)
- `costSnapshot` (phase 2)

## Step 3: default policies

For your requested behavior:

- **Scale-to-zero**: opt-in by design, but default `enabled: true` in webhook/defaulting for test environments
- **TTL** policy: mark expired first, then delete after grace period
  - safer and observable
  - avoids accidental destructive deletes
  - still deterministic cleanup

Suggested deletion sequence:

1. set condition `Expired=True`
2. emit event with reason `NamespaceExpired`
3. if grace period elapsed, delete namespace

## Step 4: reconciliation order

1. fetch `BudgetNamespace`
2. add finalizer
3. ensure namespace exists
4. apply labels/annotations
5. ensure ResourceQuota
6. ensure LimitRange
7. evaluate TTL and set status
8. evaluate over-budget policy and enforce if enabled
9. update status conditions and requeue

## Step 5: cost model for GKE (phase 2)

Use Billing Export + BigQuery as source of truth for namespace costs.

1. Enable GKE cost allocation labels on cluster.
2. Enable Cloud Billing export to BigQuery dataset.
3. Query billing export table grouped by namespace labels.
4. Reconcile compares computed period cost vs budget threshold in CR.

Important: exact namespace attribution depends on billing export labels being present and consistent.

## Step 6: enforcement recommendation

`ScaleToZero` implementation:

- list `Deployments` in namespace
- skip workloads with label `ealebed.github.io/exempt=true`
- patch replicas to `0`
- record previous replicas in annotation for optional restore

## Step 7: testing plan

- unit tests for helper functions (ttl, budget math, condition transitions)
- envtest for reconciliation behavior
- optional integration tests against a dev GKE cluster

## Step 8: delivery phases

- **Phase A (MVP)**: namespace + labels + quota + limitrange + status
- **Phase B**: ttl expiry + deletion flow + finalizer
- **Phase C**: cost ingestion from BigQuery + threshold conditions
- **Phase D**: automatic scale-to-zero enforcement
