## oms-cli init install-config

Initialize Codesphere installer configuration files

### Synopsis

Initialize config.yaml and prod.vault.yaml for the Codesphere installer.

This command generates two files:
- config.yaml: Main configuration (infrastructure, networking, plans)
- prod.vault.yaml: Secrets file (keys, certificates, passwords)

Note: When --interactive=true (default), all other configuration flags are ignored 
and you will be prompted for all settings interactively.

Supports configuration profiles for common scenarios:
- dev: Single-node development setup
- production: HA multi-node setup
- minimal: Minimal testing setup

```
oms-cli init install-config [flags]
```

### Examples

```
# Create config files interactively
$ oms-cli init install-config -c config.yaml --vault prod.vault.yaml

# Use dev profile with defaults
$ oms-cli init install-config --profile dev -c config.yaml --vault prod.vault.yaml

# Use production profile
$ oms-cli init install-config --profile production -c config.yaml --vault prod.vault.yaml

# Validate existing configuration files
$ oms-cli init install-config --validate -c config.yaml --vault prod.vault.yaml

```

### Options

```
      --acme-dns01-provider string   DNS provider for DNS-01 solver (e.g., cloudflare)
      --acme-eab-key-id string       External Account Binding key ID (required by some ACME providers)
      --acme-eab-mac-key string      External Account Binding MAC key (required by some ACME providers)
      --acme-email string            Email address for ACME account registration
      --acme-enabled                 Enable ACME certificate issuer
      --acme-issuer-name string      Name for the ACME ClusterIssuer (default "acme-issuer")
      --acme-server string           ACME server URL (default "https://acme-v02.api.letsencrypt.org/directory")
  -c, --config string                Output file path for config.yaml (default "config.yaml")
      --dc-id int                    Datacenter ID
      --dc-name string               Datacenter name
      --domain string                Main Codesphere domain
      --generate-keys                Generate SSH keys and certificates (default true)
  -h, --help                         help for install-config
      --interactive                  Enable interactive prompting (when true, other config flags are ignored) (default true)
      --k8s-control-plane strings    K8s control plane IPs (comma-separated)
      --k8s-managed                  Use Codesphere-managed Kubernetes (default true)
      --postgres-mode string         PostgreSQL setup mode (install/external)
      --postgres-primary-ip string   Primary PostgreSQL server IP
      --profile string               Use a predefined configuration profile (dev, production, minimal)
      --secrets-dir string           Secrets base directory (default "/root/secrets")
      --validate                     Validate existing config files instead of creating new ones
      --vault string                 Output file path for prod.vault.yaml (default "prod.vault.yaml")
      --with-comments                Add helpful comments to the generated YAML files
```

### SEE ALSO

* [oms-cli init](oms-cli_init.md)	 - Initialize configuration files

