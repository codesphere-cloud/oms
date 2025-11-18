# Configure the Google provider (assumes gcloud is authenticated)
terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

resource "random_id" "unique_suffix" {
  byte_length = 2 # Creates a 4-character base64 string (0-9a-f)
}

resource "google_project" "test_cluster_project" {
  # The display name uses the project_name
  name            = var.project_name

  # The Project ID combines the project_name and the random suffix 
  project_id      = "${var.project_name}-${random_id.unique_suffix.hex}"

  billing_account = var.billing_account
  auto_create_network = false

  # Conditional folder placement
  folder_id       = var.folder_id

  lifecycle {
    ignore_changes = [
      org_id,
    ]
  }
}

# 2. Enable necessary APIs
resource "google_project_service" "compute_api" {
  project = google_project.test_cluster_project.project_id
  service = "compute.googleapis.com"
  depends_on = [google_project.test_cluster_project]
}

resource "google_project_service" "service_usage_api" {
  project = google_project.test_cluster_project.project_id
  service = "serviceusage.googleapis.com"
  depends_on = [google_project.test_cluster_project]

}

resource "google_project_service" "artifact_registry_api" {
  project = google_project.test_cluster_project.project_id
  service = "artifactregistry.googleapis.com"
  depends_on = [google_project.test_cluster_project]
}
