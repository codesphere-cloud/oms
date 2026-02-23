## oms-cli install k0s

Install k0s Kubernetes distribution

### Synopsis

Install k0s either from the package or by downloading it.
This command uses k0sctl to deploy k0s clusters from a Codesphere install-config.

You must provide a Codesphere install-config file, which will:
- Generate a k0s configuration from the install-config
- Generate a k0sctl configuration for cluster deployment
- Deploy k0s to all nodes defined in the install-config using k0sctl

```
oms-cli install k0s [flags]
```

### Examples

```
# Path to Codesphere install-config file to generate k0s config from
$ oms-cli install k0s --install-config <path>

# Version of k0s to install (e.g., v1.30.0+k0s.0)
$ oms-cli install k0s --version <version>

# Version of k0sctl to use (e.g., v0.17.4)
$ oms-cli install k0s --k0sctl-version <version>

# Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from
$ oms-cli install k0s --package <file>

# SSH private key path for remote installation
$ oms-cli install k0s --ssh-key-path <path>

# Force new download and installation
$ oms-cli install k0s --force

# Skip downloading k0s binary (expects it to be on remote nodes)
$ oms-cli install k0s --no-download

```

### Options

```
  -f, --force                   Force new download and installation
  -h, --help                    help for k0s
      --install-config string   Path to Codesphere install-config file (required)
      --k0sctl-version string   Version of k0sctl to use
      --no-download             Skip downloading k0s binary
  -p, --package string          Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from
      --ssh-key-path string     SSH private key path for remote installation
  -v, --version string          Version of k0s to install
```

### SEE ALSO

* [oms-cli install](oms-cli_install.md)	 - Install Codesphere and other components

