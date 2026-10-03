## oms beta bootstrap-gcp-single-vm

Bootstrap Codesphere on one GCP Spot VM

### Synopsis

Create a single GCP Spot VM, install k0s, and run the local Codesphere bootstrap on it. For testing only.

```
oms beta bootstrap-gcp-single-vm [flags]
```

### Options

```
      --base-domain string               Base domain (Codesphere uses cs.<base-domain>)
      --billing-account string           GCP billing account ID
      --ceph-device-filter string        Rook device name filter for the extra disk (default "^sdb$")
      --config stringArray               Local config.yaml file; repeat to merge in order before VM settings
      --dns-project-id string            GCP project containing the Cloud DNS zone (default: bootstrap project)
      --dns-zone-name string             Cloud DNS zone name (default "oms-testing")
      --folder-id string                 GCP folder ID
  -h, --help                             help for bootstrap-gcp-single-vm
      --install-config-template string   Local config.yaml template to use as the base for the single VM install config
      --install-local string             Local installer-lite archive for the Codesphere installation
      --install-version string           Codesphere version to download and install
      --machine-type string              GCP machine type (default "e2-standard-8")
      --priv-key string                  Age private key for --vault (or use the age key environment variable)
      --project-name string              Unique GCP project name
      --project-ttl string               Project lifetime before cleanup (default "2h")
      --region string                    GCP region (default "europe-west4")
      --registry-user string             GHCR username for the local Codesphere installer
      --remote-oms-binary string         Local Linux amd64 OMS binary to run on the VM
      --root-disk-size int               Boot disk size in GB; an additional 100 GB Ceph disk is created (default 100)
      --ssh-private-key-path string      SSH private key path (default "~/.ssh/id_rsa")
      --ssh-public-key-path string       SSH public key path (default "~/.ssh/id_rsa.pub")
      --ssh-quiet                        Suppress SSH command output
      --storage-engine string            Storage engine for the local installer (supported: rook-ceph, local) (default "rook-ceph")
      --vault string                     SOPS vault for config templating and initial install secrets
      --zone string                      GCP zone (default "europe-west4-a")
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms beta](oms_beta.md)	 - Commands for early testing
* [oms beta bootstrap-gcp-single-vm cleanup](oms_beta_bootstrap-gcp-single-vm_cleanup.md)	 - Clean up GCP infrastructure created by bootstrap-gcp
* [oms beta bootstrap-gcp-single-vm restart-vms](oms_beta_bootstrap-gcp-single-vm_restart-vms.md)	 - Restart stopped or terminated GCP VMs

