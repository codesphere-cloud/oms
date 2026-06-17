## oms install codesphere infra

Install Codesphere infrastructure (Phase 1)

### Synopsis

Install infrastructure dependencies for a Codesphere instance (Phase 1).
Runs steps: copy-dependencies, extract-dependencies, load-container-images, sops, docker, postgres, ceph, kubernetes.

```
oms install codesphere infra [flags]
```

### Examples

```
# Install infrastructure components only
$ oms install codesphere infra -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml

# Skip loading container images when using a lite package
$ oms install codesphere infra -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images

```

### Options

```
  -h, --help   help for infra
```

### Options inherited from parent commands

```
      --auto-approve         Auto approve confirmation prompts with default values (default true)
  -c, --config string        Path to the Codesphere Private Cloud configuration file (yaml)
      --direct-connection    Use direct connection for installation, requires having access to the cluster nodes from your machine
  -f, --force                Enforce package extraction
  -p, --package string       Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
  -k, --priv-key string      Path to the private key to encrypt/decrypt secrets
  -s, --skip-steps strings   Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker
      --vault string         Path to the SOPS-encrypted prod.vault.yaml file used for config templating (default "prod.vault.yaml")
```

### SEE ALSO

* [oms install codesphere](oms_install_codesphere.md)	 - Install a Codesphere instance

