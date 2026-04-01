# Terraform for costguard-operator

This stack creates:

- VPC network and subnetwork with secondary ranges
- GKE cluster (official module)
- Artifact Registry repository for container images
- Operator GCP Service Account (GSA)
- IAM roles for billing/bigquery read
- Workload Identity IAM binding from KSA to GSA
- IAM so GKE nodes can pull from Artifact Registry
- BigQuery dataset for billing export tables

## Version policy

- Terraform CLI: `>= 1.9.0`
- Provider `hashicorp/google` and `hashicorp/google-beta`: `~> 7.25`
- GKE module: `terraform-google-modules/kubernetes-engine/google ~> 44.0`
- Network module: `terraform-google-modules/network/google ~> 16.1`

Upgrade to latest patch/minor within those major lines with:

```bash
terraform init -upgrade
```

## Usage

```bash
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars
terraform init
terraform plan
terraform apply
```

## Post-apply manual steps

1. Configure Cloud Billing export to BigQuery dataset:
   - dataset: output `billing_dataset_fqdn`
2. Cost allocation is enabled by Terraform by default via `enable_cost_allocation = true`.
   - if you disabled it, re-enable it and re-apply Terraform
3. Deploy the operator first. The Kubebuilder manifests create the operator namespace automatically.
4. After deploy, annotate the controller ServiceAccount:

```bash
kubectl annotate serviceaccount \
  -n costguard-operator-system \
  costguard-operator-controller-manager \
  iam.gke.io/gcp-service-account=<operator_gsa_email> \
  --overwrite
```

Use `operator_gsa_email` output from Terraform.

5. Restart the controller deployment so the pod picks up Workload Identity:

```bash
kubectl rollout restart deployment \
  -n costguard-operator-system \
  costguard-operator-controller-manager
```

## Image push example

Build locally and push to Artifact Registry:

```bash
export REGISTRY_HOST="$(terraform output -raw artifact_registry_docker_host)"
export PROJECT_ID="$(terraform output -raw billing_dataset_fqdn | cut -d. -f1)"
export REPOSITORY_ID="costguard-images"
export IMAGE="${REGISTRY_HOST}/${PROJECT_ID}/${REPOSITORY_ID}/costguard-operator:dev"

gcloud auth configure-docker "${REGISTRY_HOST}"
docker build -t "${IMAGE}" ..
docker push "${IMAGE}"
```
