provider "google" {
  project = var.project_id
  region  = var.region
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
}

resource "google_project_service" "required" {
  for_each = toset([
    "container.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com",
    "cloudbilling.googleapis.com",
    "bigquery.googleapis.com",
    "artifactregistry.googleapis.com",
  ])

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

module "gke" {
  source  = "terraform-google-modules/kubernetes-engine/google"
  version = "~> 44.0"

  project_id = var.project_id
  name       = var.cluster_name
  region     = var.region
  zones      = var.zones

  network    = module.vpc.network_name
  subnetwork = module.vpc.subnets_names[0]

  ip_range_pods     = var.pods_range_name
  ip_range_services = var.services_range_name

  create_service_account = true
  deletion_protection    = false
  enable_cost_allocation = true

  http_load_balancing        = true
  horizontal_pod_autoscaling = true
  network_policy             = false

  # Required for Workload Identity.
  identity_namespace = "${var.project_id}.svc.id.goog"

  node_pools = [
    {
      name            = "default-node-pool"
      machine_type    = "e2-standard-2"
      min_count       = 1
      max_count       = 3
      local_ssd_count = 0
      spot            = true
      disk_size_gb    = 50
      disk_type       = "pd-standard"
      auto_repair     = true
      auto_upgrade    = true
    }
  ]

  depends_on = [google_project_service.required]
}

module "vpc" {
  source  = "terraform-google-modules/network/google"
  version = "~> 16.1"

  project_id   = var.project_id
  network_name = var.network_name
  routing_mode = "GLOBAL"

  subnets = [
    {
      subnet_name           = var.subnet_name
      subnet_ip             = var.subnet_cidr
      subnet_region         = var.region
      subnet_private_access = true
    }
  ]

  secondary_ranges = {
    (var.subnet_name) = [
      {
        range_name    = var.pods_range_name
        ip_cidr_range = var.pods_range_cidr
      },
      {
        range_name    = var.services_range_name
        ip_cidr_range = var.services_range_cidr
      },
    ]
  }

  depends_on = [google_project_service.required]
}

resource "google_service_account" "operator_gsa" {
  account_id   = "costguard-operator"
  display_name = "Costguard Operator GSA"
  project      = var.project_id
}

# Minimal read path for billing and BigQuery exported data.
resource "google_project_iam_member" "operator_bq_data_viewer" {
  project = var.project_id
  role    = "roles/bigquery.dataViewer"
  member  = "serviceAccount:${google_service_account.operator_gsa.email}"
}

resource "google_project_iam_member" "operator_bq_job_user" {
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.operator_gsa.email}"
}

resource "google_billing_account_iam_member" "operator_billing_viewer" {
  billing_account_id = var.billing_account_id
  role               = "roles/billing.viewer"
  member             = "serviceAccount:${google_service_account.operator_gsa.email}"
}

# Workload Identity binding:
# KSA: costguard-system/costguard-controller-manager can impersonate GSA.
resource "google_service_account_iam_member" "wi_user_binding" {
  service_account_id = google_service_account.operator_gsa.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[${var.operator_namespace}/${var.operator_ksa_name}]"
}

resource "google_artifact_registry_repository" "images" {
  location      = var.region
  repository_id = var.artifact_registry_repository_id
  description   = "Docker images for costguard operator"
  format        = var.artifact_registry_format

  depends_on = [google_project_service.required]
}

resource "google_artifact_registry_repository_iam_member" "gke_pull_images" {
  location   = google_artifact_registry_repository.images.location
  repository = google_artifact_registry_repository.images.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${module.gke.service_account}"
}

# Dataset for billing export tables (export configuration is created in Billing UI/API).
resource "google_bigquery_dataset" "billing_export" {
  dataset_id    = var.billing_dataset_id
  project       = var.project_id
  location      = var.billing_dataset_location
  friendly_name = "GCP Billing Export"
  description   = "Dataset used by Cloud Billing export; read by costguard operator."

  depends_on = [google_project_service.required]
}
