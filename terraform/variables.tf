variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "billing_account_id" {
  description = "Billing account ID (format: 000000-000000-000000)"
  type        = string
}

variable "region" {
  description = "Primary GCP region"
  type        = string
  default     = "europe-west1"
}

variable "zones" {
  description = "Zones for GKE nodes"
  type        = list(string)
  default     = ["europe-west1-b", "europe-west1-c"]
}

variable "cluster_name" {
  description = "GKE cluster name"
  type        = string
  default     = "costguard-gke"
}

variable "network_name" {
  description = "VPC network name"
  type        = string
  default     = "costguard-vpc"
}

variable "subnet_name" {
  description = "Subnet name"
  type        = string
  default     = "costguard-subnet"
}

variable "subnet_cidr" {
  description = "Primary CIDR for the GKE subnet"
  type        = string
  default     = "10.10.0.0/20"
}

variable "pods_range_name" {
  description = "Secondary range name for Pods"
  type        = string
  default     = "pods-range"
}

variable "pods_range_cidr" {
  description = "Secondary CIDR range for Pods"
  type        = string
  default     = "10.20.0.0/14"
}

variable "services_range_name" {
  description = "Secondary range name for Services"
  type        = string
  default     = "services-range"
}

variable "services_range_cidr" {
  description = "Secondary CIDR range for Services"
  type        = string
  default     = "10.24.0.0/20"
}

variable "operator_namespace" {
  description = "Namespace where operator runs"
  type        = string
  default     = "costguard-operator-system"
}

variable "operator_ksa_name" {
  description = "Kubernetes ServiceAccount name for controller manager"
  type        = string
  default     = "costguard-operator-controller-manager"
}

variable "billing_dataset_id" {
  description = "BigQuery dataset for billing export"
  type        = string
  default     = "gcp_billing_export"
}

variable "billing_dataset_location" {
  description = "BigQuery dataset location for billing export (e.g. EU, US)"
  type        = string
  default     = "EU"
}

variable "artifact_registry_repository_id" {
  description = "Artifact Registry repository ID for operator images"
  type        = string
  default     = "costguard-images"
}

variable "artifact_registry_format" {
  description = "Artifact Registry repository format"
  type        = string
  default     = "DOCKER"
}
