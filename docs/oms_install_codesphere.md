## oms install codesphere

Install a Codesphere instance

### Synopsis

Install a Codesphere instance with the provided package, configuration file, and private key.
Uses the private-cloud-installer.js script included in the package to perform the installation.

```
oms install codesphere [flags]
```

### Examples

```
# Skip most pre-installation steps. E.g. if you only need to re-apply Codesphere's helm charts
$ oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s copy-dependencies,extract-dependencies,load-container-images,ceph,postgres,kubernetes,docker

# Skip loading container images. Necessary when installing a lite package that doesn't include any container images
$ oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images

```

### Options

```
  -c, --config string        Path to the Codesphere Private Cloud configuration file (yaml)
  -f, --force                Enforce package extraction
  -h, --help                 help for codesphere
  -p, --package string       Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
  -k, --priv-key string      Path to the private key to encrypt/decrypt secrets
  -s, --skip-steps strings   Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, kubernetes
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components

