output "gke_cluster_name" {
  value       = module.gke.name
  description = "Created GKE cluster name"
}

output "gke_location" {
  value       = var.region
  description = "GKE cluster region"
}

output "operator_gsa_email" {
  value       = google_service_account.operator_gsa.email
  description = "GSA for costguard operator"
}

output "workload_identity_annotation" {
  value       = "iam.gke.io/gcp-service-account: ${google_service_account.operator_gsa.email}"
  description = "Add this annotation on operator KSA"
}

output "billing_dataset_fqdn" {
  value       = "${var.project_id}.${google_bigquery_dataset.billing_export.dataset_id}"
  description = "BigQuery dataset for Cloud Billing export"
}

output "artifact_registry_repository" {
  value       = google_artifact_registry_repository.images.id
  description = "Artifact Registry repository for pushed images"
}

output "artifact_registry_docker_host" {
  value       = "${var.region}-docker.pkg.dev"
  description = "Docker registry host for Artifact Registry"
}
