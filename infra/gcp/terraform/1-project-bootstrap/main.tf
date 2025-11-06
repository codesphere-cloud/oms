# Configure the Google provider (assumes gcloud is authenticated)
terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# 1. Create the new Project
resource "google_project" "test_cluster_project" {
  name                = var.project_id
  project_id          = var.project_id
  billing_account     = var.billing_account
  auto_create_network = false
  
  # Conditional folder placement: only set if var.folder_id is provided
  folder_id           = var.folder_id 

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
