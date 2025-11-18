output "external_ips" {
  description = "The external IP addresses for jumpbox access and DNS for ingress."
value = {
    for name, instance in google_compute_instance.cluster_vms : name => instance.network_interface[0].access_config[0].nat_ip
    if length(instance.network_interface[0].access_config) > 0
  }
}

output "internal_vm_ips" {
  description = "A map of all VM hostnames to their internal IP addresses."
  value = {
    for name, instance in google_compute_instance.cluster_vms :
    name => instance.network_interface[0].network_ip
  }
}

output "artifact_registry_fqdn" {
  description = "The FQDN to use for pushing/pulling images (e.g., europe-west4-docker.pkg.dev/project-id/codesphere-test-repo)."
  value       = "${var.region}-docker.pkg.dev/${data.terraform_remote_state.bootstrap.outputs.project_id}/${google_artifact_registry_repository.codesphere_registry.repository_id}"
}

output "project_id" {
  description = "The deployed project ID."
  value       = data.terraform_remote_state.bootstrap.outputs.project_id
}

output "ar_key_base64" {
  description = "Base64 encoded JSON key for the k0s image puller service account. Use as the password."
  value       = google_service_account_key.ar_writer_key.private_key
  sensitive   = true # Mark as sensitive since it's a private key
}
