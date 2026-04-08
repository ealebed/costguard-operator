# costguard-operator

`costguard-operator` manages Kubernetes namespaces with guardrails: quota/default policies, TTL lifecycle, and budget enforcement (including scale-to-zero from BigQuery billing data).

## What it does

For each `BudgetNamespace` custom resource, the operator can:

- create/update the target `Namespace`
- apply namespace labels and annotations
- apply `ResourceQuota` (`costguard-quota`)
- apply `LimitRange` (`costguard-limitrange`)
- enforce over-budget actions by scaling workloads to zero
  - based on quota pressure (`used >= hard`)
  - based on billed spend from BigQuery (`maxSpendUSD`)
- optionally restore workloads after recovery/cooldown
- expire and delete namespaces with TTL

The operator currently enforces on:

- `Deployment`
- `StatefulSet`

## Documentation index

- [Architecture](docs/architecture.md)
- [API Reference](docs/api-reference.md)
- [Operations Runbook](docs/runbook.md)
- [Testing and Validation](docs/testing-and-validation.md)

## Quick start

### Prerequisites

- Go `1.26.1`
- Docker
- kubectl
- Terraform (if using provided infra module)
- Kubernetes cluster access (GKE recommended for cost-budget flow)

### Build and deploy

```sh
export REGISTRY_HOST="$(cd terraform && terraform output -raw artifact_registry_docker_host)"
export PROJECT_ID="$(cd terraform && terraform output -raw billing_dataset_fqdn | cut -d. -f1)"
export REPOSITORY_ID="costguard-images"
export IMG="${REGISTRY_HOST}/${PROJECT_ID}/${REPOSITORY_ID}/costguard-operator:dev"

gcloud auth configure-docker "${REGISTRY_HOST}"
make docker-build docker-push IMG="${IMG}"
make install
make deploy IMG="${IMG}"
```

### Configure BigQuery client project (recommended)

```sh
kubectl -n costguard-operator-system set env \
  deploy/costguard-operator-controller-manager \
  GOOGLE_CLOUD_PROJECT=ylebi-rnd
kubectl -n costguard-operator-system rollout status deploy/costguard-operator-controller-manager
```

### Apply sample CRs

```sh
kubectl apply -k config/samples/
```

## Uninstall

```sh
kubectl delete -k config/samples/
make uninstall
make undeploy
```

## Development

Common targets:

- `make test`
- `make lint`
- `make build`
- `make run`
- `make deploy IMG=<image>`

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0.
