## oms beta bootstrap-gcp single-node

Bootstrap k0s and Codesphere on one GCP VM

### Synopsis

Install k0s and Codesphere on one GCP VM using the local bootstrap.
Requires --registry-user, either --github-pat or OMS_REGISTRY_PASSWORD,
and either --install-local or --install-version. The root disk defaults to 50 GB;
a separate 100 GB Ceph disk is added. Supply installation settings through --config and --vault.
Not for production use.

```
oms beta bootstrap-gcp single-node [flags]
```

### Options

```
      --base-domain string               Base domain for Codesphere (required)
      --billing-account string           GCP Billing Account ID (required)
      --config stringArray               Local config.yaml file; repeat to merge in order before VM settings
      --dns-project-id string            GCP Project ID for Cloud DNS (optional)
      --dns-zone-name string             Cloud DNS Zone Name (optional) (default "oms-testing")
      --folder-id string                 GCP Folder ID (optional)
      --github-pat string                GitHub Personal Access Token for pulling Codesphere images from ghcr.io (defaults to OMS_REGISTRY_PASSWORD). Required scope: read:packages.
  -h, --help                             help for single-node
      --install-config-template string   Initial config.yaml template, applied before --config files
      --install-hash string              Codesphere package hash to install (default: none)
      --install-local string             Install Codesphere from local package (default: none)
      --install-version string           Codesphere version to install (default: none)
      --machine-type string              GCP machine type override for all VMs (default: role-specific sizes; e2-standard-8 for a single VM)
      --priv-key string                  Age private key for --vault (or use the age key environment variable)
      --project-name string              Unique GCP Project Name (required)
      --project-ttl string               Time to live for the GCP project. Cleanup workflows will remove it afterwards. (default: 2 hours) (default "2h")
      --region string                    GCP Region (default: europe-west4) (default "europe-west4")
      --registry-user string             Registry username for pulling Codesphere images from ghcr.io (required)
      --remote-oms-binary string         Path to a local Linux amd64 OMS binary to copy to and use on the jumpbox instead of downloading a release (optional)
      --root-disk-size int               Instance root disk size in GB (default: 50) (default 50)
      --spot-vms                         Use Spot VMs for Codesphere infrastructure. Falls back to standard VMs if spot capacity unavailable. (default: false)
      --ssh-private-key-path string      SSH Private Key Path (default: ~/.ssh/id_rsa) (default "~/.ssh/id_rsa")
      --ssh-public-key-path string       SSH Public Key Path (default: ~/.ssh/id_rsa.pub) (default "~/.ssh/id_rsa.pub")
      --ssh-quiet                        Suppress SSH command output (default: false)
      --storage-engine string            Storage engine (rook-ceph or local) (default "rook-ceph")
      --timer                            Show how long each step took and a final timing summary table
      --vault string                     SOPS vault for config templating and initial install secrets
      --zone string                      GCP Zone (default: europe-west4-a) (default "europe-west4-a")
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms beta bootstrap-gcp](oms_beta_bootstrap-gcp.md)	 - Bootstrap GCP infrastructure for Codesphere

