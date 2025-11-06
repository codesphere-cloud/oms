variable "region" {
  description = "The GCP region to deploy resources into"
  type        = string
  default     = "europe-west4" # Cost-effective European region (Netherlands)
}

variable "zone" {
  description = "The GCP zone to deploy resources into"
  type        = string
  default     = "europe-west4-a"
}

variable "ssh_public_key_path" {
  description = "Path to the public SSH key file (e.g., ~/.ssh/id_rsa.pub)"
  type        = string
}

variable "vm_scheduling_type" {
  description = "The VM scheduling type: 'SPOT' for preemptible, or 'ON_DEMAND' for standard."
  type        = string
  default     = "SPOT" # Defaulting to the cheaper SPOT (Preemptive) option
  validation {
    condition     = contains(["SPOT", "ON_DEMAND"], upper(var.vm_scheduling_type))
    error_message = "vm_scheduling_type must be either 'SPOT' or 'ON_DEMAND'."
  }
}

variable "project_name" {
  description = "The GCP project name used for state file naming"
  type        = string
}
