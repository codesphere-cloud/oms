## oms init install-config

Initialize Codesphere installer configuration files

### Synopsis

Initialize config.yaml and prod.vault.yaml for the Codesphere installer.

This command generates two files:
- config.yaml: Main configuration (infrastructure, networking, plans)
- prod.vault.yaml: Secrets file (keys, certificates, passwords)

Note: When --interactive=true (default), all other configuration flags are ignored 
and you will be prompted for all settings interactively.

Note: When using ansible-inventory make sure the inventory follows our supported structure.
Supported YAML format (where 'hosts' is a dictionary of hostname keys):
- <k8s-cp|k8s-workers|ceph>.hosts.<hostname>.private_ip

Supports configuration profiles for common scenarios:
- dev: Single-node development setup
- production: HA multi-node setup
- minimal: Minimal testing setup


```
oms init install-config [flags]
```

### Examples

```
# Create config files interactively
$ oms init install-config -c config.yaml --vault prod.vault.yaml

# Use dev profile with defaults
$ oms init install-config --profile dev -c config.yaml --vault prod.vault.yaml

# Use production profile
$ oms init install-config --profile production -c config.yaml --vault prod.vault.yaml

# Use ansible inventory for host definitions
$ oms init install-config --profile production -c config.yaml --ansible-inventory inventory.yaml

# Validate existing configuration files
$ oms init install-config --validate -c config.yaml --vault prod.vault.yaml

```

### Options

```
      --acme-dns01-provider string    DNS provider for DNS-01 solver (e.g., cloudflare)
      --acme-eab-key-id string        External Account Binding key ID (required by some ACME providers)
      --acme-eab-mac-key string       External Account Binding MAC key (required by some ACME providers)
      --acme-email string             Email address for ACME account registration
      --acme-enabled                  Enable ACME certificate issuer
      --acme-issuer-name string       Name for the ACME ClusterIssuer (default "acme-issuer")
      --acme-server string            ACME server URL (default "https://acme-v02.api.letsencrypt.org/directory")
      --ansible-inventory string      Path to Ansible inventory file to import host information from
      --ceph-csi-kubelet-dir string   Directory of kubelet for ceph csi. Required for some cloud providers
      --ceph-nodes-subnet string      CIDR subnet for ceph nodes
  -c, --config string                 Output file path for config.yaml (default "config.yaml")
      --dc-city string                Datacenter city
      --dc-country-code string        Datacenter country code
      --dc-id int                     Datacenter ID
      --dc-name string                Datacenter name
      --domain string                 Main Codesphere domain
      --generate-keys                 Generate SSH keys and certificates (default true)
  -h, --help                          help for install-config
      --interactive                   Enable interactive prompting (when true, other config flags are ignored) (default true)
      --k8s-control-plane strings     K8s control plane IPs (comma-separated)
      --k8s-managed                   Use Codesphere-managed Kubernetes (default true)
      --openbao-engine string         Engine for OpenBao (default "cs-secrets-engine")
      --openbao-password string       Password for OpenBao authentication
      --openbao-uri string            URI for OpenBao (e.g., https://openbao.example.com)
      --openbao-user string           Username for OpenBao authentication (default "admin")
      --postgres-mode string          PostgreSQL setup mode (install/external)
      --postgres-primary-ip string    Primary PostgreSQL server IP
      --postgres-server string        PostgreSQL server hostname for install mode or address for external mode
      --profile string                Use a predefined configuration profile (dev, production, minimal)
      --registry-server string        Server for container registry
      --secrets-dir string            Secrets base directory (default "/root/secrets")
      --validate                      Validate existing config files instead of creating new ones
      --vault string                  Output file path for prod.vault.yaml (default "prod.vault.yaml")
      --with-comments                 Add helpful comments to the generated YAML files
```

### SEE ALSO

* [oms init](oms_init.md)	 - Initialize configuration files
