# Testing and Validation

## Test strategy

The project uses:

- unit tests (`internal/..._test.go`)
- controller tests with envtest (`internal/controller/..._test.go`)
- optional e2e flow (`test/e2e`)

Run all non-e2e tests:

```sh
go test ./... -count=1
```

Run make pipeline:

```sh
make test
make lint
```

## Functional validation flows

## 1) Quota-based scale-to-zero

1. Apply `config/samples/finops_v1alpha1_budgetnamespace_quota_demo.yaml`.
2. Create workload in managed namespace with enough requests to hit quota.
3. Observe:
   - `OverBudget=True` with quota reason
   - replicas scaled to zero

## 2) Cost-budget scale-to-zero

1. Apply `config/samples/finops_v1alpha1_budgetnamespace_cost_budget.yaml`.
2. Ensure:
   - billing table and cluster name are correct
   - billing location matches dataset (e.g. EU)
   - controller has `GOOGLE_CLOUD_PROJECT`
3. Observe status:

```sh
kubectl get budgetnamespace budgetnamespace-costalloc -n default -o yaml | \
  rg "lastObservedSpendUSD|lastCostQueryAt|OverBudget|Reason|Message"
```

4. Observe workloads:

```sh
kubectl get deploy,statefulset -n costalloc-test-ns -w
```

## 3) Fast forced validation

Temporarily reduce threshold and interval:

```sh
kubectl patch budgetnamespace budgetnamespace-costalloc -n default --type='merge' \
  -p '{"spec":{"costBudget":{"maxSpendUSD":"0.0025","queryInterval":"1m"}}}'

kubectl annotate budgetnamespace budgetnamespace-costalloc -n default \
  finops.ealebed.github.io/force-reconcile="$(date +%s)" --overwrite
```

Expected:

- deployment scales to `0/0`
- `OverBudget` reason becomes `CostBudgetExceeded`
- controller logs `ScaledToZero`

## 4) Restore path validation

With `restoreOnRecovery=true`:

1. After scale-down, set budget below current spend condition to false (raise threshold or reduce spend window).
2. Wait cooldown.
3. Verify replicas restored from annotation.

## Debug commands

```sh
kubectl -n costguard-operator-system logs deploy/costguard-operator-controller-manager --since=20m | \
  rg "ScaledToZero|RestoredReplicas|CostBudget|OverBudget|error"

kubectl get budgetnamespace -A
kubectl describe budgetnamespace budgetnamespace-costalloc -n default
```
