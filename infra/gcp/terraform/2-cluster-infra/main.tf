# Configure the Google provider and link to the project
terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# Read the state file from the bootstrap folder to get the project ID
# Note: This path will be dynamically resolved based on the PROJECT_NAME variable
data "terraform_remote_state" "bootstrap" {
  backend = "local"
  config = {
    path = "../1-project-bootstrap/${var.project_name}.tfstate"
  }
}

# The data "google_compute_subnetwork" "default_subnet" block has been removed
# as it caused timing errors with auto_create_subnetworks.

provider "google" {
  project = data.terraform_remote_state.bootstrap.outputs.project_id
  region  = var.region
}

# 1. Define common locals
locals {
  # Format SSH key for metadata: "user:key-content"
  SSH_PUBLIC_KEY_CONTENT = trimspace(file(var.ssh_public_key_path))

  ssh_key_entry = <<-EOT
root:${local.SSH_PUBLIC_KEY_CONTENT}
ubuntu:${local.SSH_PUBLIC_KEY_CONTENT}
EOT

  ceph_vms = {
    for i in range(1, 5) : "ceph-${i}" =>  {
      machine_type= "n2-standard-8"
      disk_sizes   = { root = 100, mds = 50, data = 500 }
      external_ip  = false
    }
  }

  k0s_vms = {
    for i in range(1, 4) : "k0s-cp-${i}" => {
      machine_type = "n2-standard-8"
      disk_sizes   = { root = 100 }
      external_ip  = true
    }
  }

  other_vms = {
    "jumpbox" = {
      machine_type = "n2-standard-2"
      disk_sizes   = { root = 100 }
      external_ip  = true
    }

    "postgres" = {
      machine_type = "n2-standard-4"
      disk_sizes   = { root = 100 }
      external_ip  = true
    }
  }

  vms = merge(local.ceph_vms, local.k0s_vms, local.other_vms)
}

# 2. Networking (VPC and Firewall)
resource "google_compute_network" "vpc" {
  name                    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "cluster_subnet" {
  name          = "${data.terraform_remote_state.bootstrap.outputs.project_id}-${var.region}-subnet"
  ip_cidr_range = "10.10.0.0/20" # Explicitly define a private CIDR block
  region        = var.region
  network       = google_compute_network.vpc.self_link
}

resource "google_compute_router" "nat_router" {
  name    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-router"
  project = data.terraform_remote_state.bootstrap.outputs.project_id
  region  = var.region
  network = google_compute_network.vpc.self_link
}

resource "google_compute_router_nat" "cluster_nat" {
  name                               = "${data.terraform_remote_state.bootstrap.outputs.project_id}-nat-gateway"
  router                             = google_compute_router.nat_router.name
  region                             = google_compute_router.nat_router.region
  
  # Configuration for the NAT IP addresses
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
  nat_ip_allocate_option             = "AUTO_ONLY"

  # Configuration for the internal hosts to NAT
  log_config {
    enable = false
    filter = "ERRORS_ONLY"
  }
}

# Firewall rule to allow external SSH only to the Jumpbox
resource "google_compute_firewall" "allow_ssh_ext" {
  name    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-allow-ssh-ext"
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["ssh-external"]
}

# Firewall rule to allow all communication within the private network
resource "google_compute_firewall" "allow_internal" {
  name    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-allow-internal"
  network = google_compute_network.vpc.name

  # Allow all traffic within the VPC for Ceph/k0s communication
  allow {
    protocol = "all"
  }

  source_ranges = [google_compute_subnetwork.cluster_subnet.ip_cidr_range]
}

resource "google_compute_firewall" "allow_all_egress" {
  name    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-allow-all-egress"
  network = google_compute_network.vpc.name
  direction = "EGRESS"

  allow {
    protocol = "all"
  }

  destination_ranges = ["0.0.0.0/0"]
}

resource "google_compute_firewall" "allow_ingress_web" {
  name    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-allow-web"
  network = google_compute_network.vpc.name
  
  direction = "INGRESS" # Inbound traffic

  allow {
    protocol = "tcp"
    ports    = ["80", "443"] # Allow standard HTTP and HTTPS ports
  }

  # Allow traffic from all destinations (the Internet)
  source_ranges = ["0.0.0.0/0"] 

  # Apply this rule to all VMs with external access (Jumpbox and k0s nodes)
  # This tag was already set on your k0s VMs and Jumpbox when they got their external IPs.
  target_tags = ["ssh-external"] 
}

# Allow external access to PostgreSQL on the postgres VM
resource "google_compute_firewall" "allow_ingress_postgres" {
  name    = "${data.terraform_remote_state.bootstrap.outputs.project_id}-allow-postgres"
  network = google_compute_network.vpc.name
  direction = "INGRESS"
  source_ranges = ["0.0.0.0/0"] 

  allow {
    protocol = "tcp"
    ports    = ["5432"] # PostgreSQL default port
  }

  target_tags = ["postgres-external"] 
}

# 3. Artifact Registry Setup (Managed Registry)
resource "google_artifact_registry_repository" "codesphere_registry" {
  location      = var.region
  repository_id = "codesphere-test-repo"
  description   = "Docker repository for Codesphere test images"
  format        = "DOCKER"
}

resource "google_service_account" "ar_sa" {
  project      = data.terraform_remote_state.bootstrap.outputs.project_id
  account_id   = "k0s-ar-sa"
  display_name = "K0s Artifact Registry Account"
}

resource "google_project_iam_member" "ar_writer" {
  project = data.terraform_remote_state.bootstrap.outputs.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.ar_sa.email}"
}
resource "google_service_account_key" "ar_writer_key" {
  service_account_id = google_service_account.ar_sa.name
}

# 4. Data Disks (Ceph MDS and Ceph Data)

# Ceph MDS Disks (50GiB SSD)
resource "google_compute_disk" "ceph_mds" {
  # Use lookup to safely check if 'mds' size exists; only include if the result is not null
  for_each = {
    for k, v in local.vms : k => v
    if lookup(v.disk_sizes, "mds", null) != null
  }
  name     = "${each.key}-mds-disk"
  type     = "pd-ssd"
  zone     = var.zone
  size     = each.value.disk_sizes.mds
}

# Ceph Data Disks (500GiB SSD)
resource "google_compute_disk" "ceph_data" {
  # Use lookup to safely check if 'data' size exists; only include if the result is not null
  for_each = {
    for k, v in local.vms : k => v
    if lookup(v.disk_sizes, "data", null) != null
  }
  name     = "${each.key}-data-disk"
  type     = "pd-ssd"
  zone     = var.zone
  size     = each.value.disk_sizes.data
}


# 5. Compute Instances (9 VMs)
resource "google_compute_instance" "cluster_vms" {
  for_each     = local.vms
  name         = each.key
  machine_type = each.value.machine_type
  zone         = var.zone

  # 5a. Conditional Scheduling
  scheduling {
    preemptible       = var.vm_scheduling_type == "SPOT"
    automatic_restart = var.vm_scheduling_type == "ON_DEMAND"
  }

  # 5b. Network Interface
  network_interface {
    subnetwork = google_compute_subnetwork.cluster_subnet.self_link

    # Only assign an external IP to the jumpbox
    dynamic "access_config" {
      for_each = each.value.external_ip == true ? [1] : []
      content {
        network_tier = "STANDARD"
      }
    }
  }

  # 5c. Boot Disk (100GiB SSD for all)
  boot_disk {
    initialize_params {
      image = "ubuntu-os-cloud/ubuntu-2204-lts"
      size  = each.value.disk_sizes.root
      type  = "pd-ssd"
    }
  }

  # 5d. Attach Disks
  dynamic "attached_disk" {
    for_each = lookup(each.value.disk_sizes, "mds", null) != null ? [1] : []
    content {
      source = google_compute_disk.ceph_mds[each.key].self_link
    }
  }

  dynamic "attached_disk" {
    for_each = lookup(each.value.disk_sizes, "data", null) != null ? [1] : []
    content {
      source = google_compute_disk.ceph_data[each.key].self_link
    }
  }

  # 5e. SSH Key and Tags
  metadata = {
    ssh-keys = local.ssh_key_entry
  }

  tags = concat(
    ["all-vms"], 
    each.value.external_ip == true ? ["ssh-external"] : [],
    each.key == "postgres" ? ["postgres-external"] : []
  )
}
