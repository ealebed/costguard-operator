# Operations Runbook

## Day-0 deployment

1. Build and push image.
2. `make install` and `make deploy IMG=<image>`.
3. Configure controller env for BigQuery project:

```sh
kubectl -n costguard-operator-system set env \
  deploy/costguard-operator-controller-manager \
  GOOGLE_CLOUD_PROJECT=ylebi-rnd
kubectl -n costguard-operator-system rollout status deploy/costguard-operator-controller-manager
```

4. Ensure Workload Identity / IAM permissions for BigQuery read and jobs.

## Day-1 operations

### Check controller health

```sh
kubectl -n costguard-operator-system get pods
kubectl -n costguard-operator-system logs deploy/costguard-operator-controller-manager --since=15m
```

### Check BudgetNamespace state

```sh
kubectl get budgetnamespace -A
kubectl get budgetnamespace <name> -n <ns> -o yaml
```

### Key status fields

- `status.conditions[type=OverBudget]`
- `status.lastCostQueryAt`
- `status.lastObservedSpendUSD`
- `status.lastEnforcementOperation`

## Common tasks

### Force reconcile

```sh
kubectl annotate budgetnamespace <name> -n <ns> finops.ealebed.github.io/force-reconcile="$(date +%s)" --overwrite
```

### Temporarily lower threshold for validation

```sh
kubectl patch budgetnamespace <name> -n <ns> --type='merge' \
  -p '{"spec":{"costBudget":{"maxSpendUSD":"0.0025","queryInterval":"1m"}}}'
```

### Revert threshold

```sh
kubectl patch budgetnamespace <name> -n <ns> --type='merge' \
  -p '{"spec":{"costBudget":{"maxSpendUSD":"0.03","queryInterval":"15m"}}}'
```

## Troubleshooting

### Symptom: BigQuery `404 notFound`

Checklist:

- `spec.costBudget.billingLocation` matches dataset location (`EU`/`US`)
- `spec.costBudget.billingExportTable` is exact
- controller has `GOOGLE_CLOUD_PROJECT`
- controller image includes latest fixes

Commands:

```sh
kubectl get budgetnamespace <name> -n <ns> -o yaml | rg "billingExportTable|billingLocation|clusterName"
kubectl -n costguard-operator-system get deploy costguard-operator-controller-manager -o jsonpath='{.spec.template.spec.containers[0].env}'
kubectl -n costguard-operator-system logs deploy/costguard-operator-controller-manager --since=20m | rg "BigQuery|CostBudget|notFound|error"
```

### Symptom: no scale-down even though feature enabled

- verify `OverBudget` condition and reason
- verify `lastObservedSpendUSD` < or >= threshold
- verify workload is not exempt (`ealebed.github.io/exempt=true`)
- verify managed resources are `Deployment`/`StatefulSet`

### Symptom: restore did not happen

- check `restoreOnRecovery=true`
- check cooldown not elapsed
- check spend below threshold (if costBudget enabled)

## Safety guidance

- Start with small test namespace and low thresholds.
- Use `restoreOnRecovery: true` for non-destructive validation loops.
- Keep realistic production thresholds and larger windows.
