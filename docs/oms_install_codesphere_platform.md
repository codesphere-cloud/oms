## oms install codesphere platform

Install the Codesphere platform (Phase 3)

### Synopsis

Install the Codesphere platform (Phase 3).
Runs step: codesphere.
Requires the infrastructure and dependencies phases to have completed successfully.

```
oms install codesphere platform [flags]
```

### Examples

```
# Install Codesphere platform only
$ oms install codesphere platform -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml

```

### Options

```
      --auto-approve         Auto approve confirmation prompts with default values (default true)
  -c, --config string        Path to the Codesphere Private Cloud configuration file (yaml)
      --direct-connection    Use direct connection for installation, requires having access to the cluster nodes from your machine
  -f, --force                Enforce package extraction
  -h, --help                 help for platform
  -p, --package string       Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
  -k, --priv-key string      Path to the private key to encrypt/decrypt secrets
  -s, --skip-steps strings   Platform steps to skip. E.g. codesphere
      --vault string         Path to the SOPS-encrypted prod.vault.yaml file used for config templating (default "prod.vault.yaml")
```

### SEE ALSO

* [oms install codesphere](oms_install_codesphere.md)	 - Install a Codesphere instance

