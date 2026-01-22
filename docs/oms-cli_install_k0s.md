## oms-cli install k0s

Install k0s Kubernetes distribution

### Synopsis

Install k0s either from the package or by downloading it.
This will either download the k0s binary directly to the OMS workdir, if not already present, and install it
or load the k0s binary from the provided package file and install it.
If no version is specified, the latest version will be downloaded.

You must provide a Codesphere install-config file, which will:
- Generate a k0s configuration from the install-config
- Optionally install k0s on remote nodes via SSH

```
oms-cli install k0s [flags]
```

### Examples

```
# Path to Codesphere install-config file to generate k0s config from
$ oms-cli install k0s --install-config <path>

# Version of k0s to install
$ oms-cli install k0s --version <version>

# Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from
$ oms-cli install k0s --package <file>

# Remote host IP to install k0s on (requires --ssh-key-path)
$ oms-cli install k0s --remote-host <ip>

# SSH private key path for remote installation
$ oms-cli install k0s --ssh-key-path <path>

# Force new download and installation even if k0s binary exists or is already installed
$ oms-cli install k0s --force

```

### Options

```
  -f, --force                   Force new download and installation
  -h, --help                    help for k0s
      --install-config string   Path to Codesphere install-config file (required)
  -p, --package string          Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from
      --remote-host string      Remote host IP to install k0s on
      --remote-user string      Remote user for SSH connection (default "root")
      --ssh-key-path string     SSH private key path for remote installation
  -v, --version string          Version of k0s to install
```

### SEE ALSO

* [oms-cli install](oms-cli_install.md)	 - Install Codesphere and other components

