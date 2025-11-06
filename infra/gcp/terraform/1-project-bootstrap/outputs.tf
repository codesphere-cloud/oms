output "project_id" {
  description = "The ID of the newly created project"
  value       = google_project.test_cluster_project.project_id
}
