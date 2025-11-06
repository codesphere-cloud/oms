variable "billing_account" {
  description = "Your GCP billing account ID"
  type        = string
}

variable "project_name" {
  description = "The project name for the new GCP project"
  type        = string
}

variable "folder_id" {
  description = "The numeric ID of the folder (e.g., 1234567890) this project should be created under. Omit if creating directly under an Organization."
  type        = string
  default     = null # Making it optional
}
