## oms-cli beta bootstrap-gcp

Bootstrap GCP infrastructure for Codesphere

### Synopsis

Bootstraps GCP infrastructure required to run Codesphere clusters on GCP.
This includes setting up projects, service accounts, and necessary IAM roles.
Depending on your setup, additional configuration may be required after bootstrapping.
Ensure you have the necessary permissions to create and manage GCP resources before proceeding.
Not for production use.

```
oms-cli beta bootstrap-gcp [flags]
```

### Options

```
      --base-domain string                Base domain for Codesphere (required)
      --billing-account string            GCP Billing Account ID (required)
      --custom-pg-ip string               Custom PostgreSQL IP (optional)
      --datacenter-id int                 Datacenter ID (default: 1) (default 1)
      --dns-project-id string             GCP Project ID for Cloud DNS (optional)
      --dns-project-sa string             GCP Project Service Account for Cloud DNS (optional)
      --dns-zone-name string              Cloud DNS Zone Name (optional) (default "oms-testing")
      --experiments stringArray           Experiments to enable in Codesphere installation (optional) (default [managed-services,vcluster,custom-service-image,ms-in-ls,secret-management,sub-path-mount])
      --feature-flags stringArray         Feature flags to enable in Codesphere installation (optional)
      --folder-id string                  GCP Folder ID (optional)
      --github-app-client-id string       Github App Client ID (required)
      --github-app-client-secret string   Github App Client Secret (required)
      --github-pat string                 GitHub Personal Access Token to use for direct image access. Scope required: package read (optional)
  -h, --help                              help for bootstrap-gcp
      --install-config string             Path to install config file (optional) (default "config.yaml")
      --install-hash string               Codesphere package hash to install (default: none)
  -s, --install-skip-steps stringArray    Installation steps to skip during Codesphere installation (optional)
      --install-version string            Codesphere version to install (default: none)
      --preemptible                       Use preemptible VMs for Codesphere infrastructure (default: false)
      --project-name string               Unique GCP Project Name (required)
      --region string                     GCP Region (default: europe-west4) (default "europe-west4")
      --registry-type string              Container registry type to use (options: local-container, artifact-registry) (default: artifact-registry) (default "local-container")
      --registry-user string              Custom Registry username (only for GitHub registry type) (optional)
      --secrets-dir string                Directory for secrets (default: /etc/codesphere/secrets) (default "/etc/codesphere/secrets")
      --secrets-file string               Path to secrets files (optional) (default "prod.vault.yaml")
      --ssh-private-key-path string       SSH Private Key Path (default: ~/.ssh/id_rsa) (default "~/.ssh/id_rsa")
      --ssh-public-key-path string        SSH Public Key Path (default: ~/.ssh/id_rsa.pub) (default "~/.ssh/id_rsa.pub")
      --ssh-quiet                         Suppress SSH command output (default: true) (default true)
      --write-config                      Write generated install config to file (default: true) (default true)
      --zone string                       GCP Zone (default: europe-west4-a) (default "europe-west4-a")
```

### SEE ALSO

* [oms-cli beta](oms-cli_beta.md)	 - Commands for early testing
* [oms-cli beta bootstrap-gcp postconfig](oms-cli_beta_bootstrap-gcp_postconfig.md)	 - Run post-configuration steps for GCP bootstrapping

