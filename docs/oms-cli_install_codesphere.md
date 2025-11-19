## oms-cli install codesphere

Install a Codesphere instance

### Synopsis

Install a Codesphere instance with the provided package, configuration file, and private key.
Uses the private-cloud-installer.js script included in the package to perform the installation.

```
oms-cli install codesphere [flags]
```

### Options

```
  -c, --config string        Path to the Codesphere Private Cloud configuration file (yaml)
  -f, --force                Enforce package extraction
  -h, --help                 help for codesphere
  -p, --package string       Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
  -k, --priv-key string      Path to the private key to encrypt/decrypt secrets
  -s, --skip-steps strings   Steps to be skipped. Must be one of: copy-dependencies, extract-dependencies, load-container-images, ceph, kubernetes
```

### SEE ALSO

* [oms-cli install](oms-cli_install.md)	 - Coming soon: Install Codesphere and other components

