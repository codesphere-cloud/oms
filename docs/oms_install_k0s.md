## oms install k0s

Install k0s Kubernetes distribution

### Synopsis

Install k0s either from the package or by downloading it.
This command uses k0sctl to deploy k0s clusters from a Codesphere install-config.
With --single, install a controller and worker on this machine without k0sctl or an install-config.

For multi-node deployment, provide a Codesphere install-config file, which will:
- Generate a k0s configuration from the install-config
- Generate a k0sctl configuration for cluster deployment
- Deploy k0s to all nodes defined in the install-config using k0sctl

An existing generated k0sctl config is only overwritten after confirmation or with --yes.

Alternatively, apply a k0sctl config created with 'oms init k0s-config' with --config.

```
oms install k0s [flags]
```

### Examples

```
# Path to Codesphere install-config file to generate k0s config from
$ oms install k0s --install-config <path>

# Overwrite a previously generated k0sctl config without asking
$ oms install k0s --install-config <path> --yes

# Path to an existing k0sctl config to apply
$ oms install k0s --config <path>

# Version of k0s to install (e.g., v1.36.5+k0s.0)
$ oms install k0s --version <version>

# Version of k0sctl to use (e.g., v0.17.4)
$ oms install k0s --k0sctl-version <version>

# Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from
$ oms install k0s --package <file>

# SSH private key path for remote installation
$ oms install k0s --ssh-key-path <path>

# Force new download and installation
$ oms install k0s --force

# Skip downloading k0s binary (expects it to be on remote nodes)
$ oms install k0s --no-download

# Install a single-node cluster on this machine using bundled k0s
$ oms install k0s --single --package <file>

```

### Options

```
      --config string           Path to an existing k0sctl config to apply, e.g. created with 'oms init k0s-config'
  -f, --force                   Force new download and installation
  -h, --help                    help for k0s
      --install-config string   Path to Codesphere install-config file to generate the k0sctl config from
      --k0sctl-version string   Version of k0sctl to use (default "v0.33.1")
      --no-download             Skip downloading k0s binary
  -p, --package string          Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from
      --single                  Install a single-node controller and worker on this machine
      --ssh-key-path string     SSH private key path for remote installation
      --vault string            Path to prod.vault.yaml to save the kubeconfig into (optional)
      --vault-priv-key string   Path to the age private key to decrypt the vault (optional, for SOPS-encrypted vaults)
      --vault-type string       Vault storage type (sops or plain) (default "sops")
  -v, --version string          Version of k0s to install (default "v1.36.5+k0s.0")
  -y, --yes                     Overwrite an existing generated k0sctl config without asking
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components

