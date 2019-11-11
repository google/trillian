variable "gcp_project" {
  type = string
}

variable "region" {
  type    = string
  default = "us-west1"
}

provider "google" {
  project = var.gcp_project
  version = "~> v3.0.0-beta.1"
}

provider "google-beta" {
  project = var.gcp_project
  version = "~> v3.0.0-beta.1"
}

# Enable required API in the project

resource "google_project_service" "container-api" {
  service = "container.googleapis.com"
}

resource "google_project_service" "spanner-api" {
  service = "spanner.googleapis.com"
}

# Main cluster definition

## Force recent version for Kubernetes master
data "google_container_engine_versions" "gke-ver" {
  location       = var.region
  version_prefix = "1.14."
}

resource "google_container_cluster" "trillian-cluster" {
  provider           = google-beta
  name               = "cluster"
  location           = var.region
  node_version       = data.google_container_engine_versions.gke-ver.latest_node_version
  min_master_version = data.google_container_engine_versions.gke-ver.latest_node_version

  initial_node_count = 3

  node_config {
    machine_type = "n1-standard-2"
    image_type   = "COS"

    workload_metadata_config {
      node_metadata = "GKE_METADATA_SERVER"
    }
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/devstorage.read_only"
    ]
  }

  workload_identity_config {
    identity_namespace = "${var.gcp_project}.svc.id.goog"
  }

  depends_on = [
    google_project_service.container-api
  ]
}

# Spanner instance

locals {
  trillian_ddl = file("${path.module}/../storage/cloudspanner/spanner.sdl")
}

resource "google_spanner_instance" "trillian-spanner" {
  name         = "trillian-spanner"
  display_name = "Trillian Spanner Instance"
  config       = "regional-${var.region}"
  num_nodes    = 1
  depends_on = [
    google_project_service.spanner-api
  ]
}

resource "google_spanner_database" "trillian-db" {
  instance = google_spanner_instance.trillian-spanner.name
  name     = "trillian-db"
  # Format the DDL (remove comment and split the lines)
  ddl = split(";", replace(replace(local.trillian_ddl, "/--.*\\n/", ""), "\n", ""))
}

# Create a GCP service account, give it access to Spanner and add a binding so that a
# kubernetes service account can use that account.

resource "google_service_account" "trillian" {
  account_id   = "trillian"
  display_name = "Trillian service account"
}

resource "google_project_iam_binding" "trillian-sc-dbuser" {
  role = "roles/spanner.databaseUser"
  members = [
    "serviceAccount:${google_service_account.trillian.email}",
  ]
}

resource "google_project_iam_binding" "trillian-sc-logwriter" {
  role = "roles/logging.logWriter"
  members = [
    "serviceAccount:${google_service_account.trillian.email}",
  ]
}

resource "google_project_iam_binding" "trillian-sc-metricwriter" {
  role = "roles/monitoring.metricWriter"
  members = [
    "serviceAccount:${google_service_account.trillian.email}",
  ]
}

resource "google_service_account_iam_binding" "trillian-sc-identity" {
  service_account_id = google_service_account.trillian.name
  role               = "roles/iam.workloadIdentityUser"

  members = [
    "serviceAccount:${var.gcp_project}.svc.id.goog[default/trillian-svc]",
  ]
  depends_on = [
    google_container_cluster.trillian-cluster
  ]
}

output "next_steps" {
  value       = <<EOT

gcloud config set project ${var.gcp_project} && \
gcloud container clusters get-credentials cluster --region=${var.region}

	EOT
  description = "Command to configure Kubernetes"
}

resource "local_file" "tf-patch" {
  content  = templatefile("tf-patch.template.yaml", { project_id = var.gcp_project })
  filename = "tf-patch.yaml"
}
