## oms install k0s

Install k0s Kubernetes distribution

### Synopsis

Install k0s either from the package or by downloading it.
This will either download the k0s binary directly to the OMS workdir, if not already present, and install it
or load the k0s binary from the provided package file and install it.
If no version is specified, the latest version will be downloaded.
If no install config is provided, k0s will be installed with the '--single' flag.

```
oms install k0s [flags]
```

### Examples

```
# Install k0s using the Go-native implementation
$ oms install k0s

# Version of k0s to install
$ oms install k0s --version <version>

# Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from
$ oms install k0s --package <file>

# Path to k0s configuration file, if not set k0s will be installed with the '--single' flag
$ oms install k0s --k0s-config <path>

# Force new download and installation even if k0s binary exists or is already installed
$ oms install k0s --force

```

### Options

```
  -f, --force               Force new download and installation
  -h, --help                help for k0s
      --k0s-config string   Path to k0s configuration file
  -p, --package string      Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from
  -v, --version string      Version of k0s to install
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components

