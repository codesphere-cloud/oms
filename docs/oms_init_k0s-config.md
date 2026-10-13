## oms init k0s-config

Generate a k0sctl config for installing k0s

### Synopsis

Generate a k0sctl configuration from a Codesphere install-config without
installing k0s.

With --airgapped, the config uploads the k0s airgap image bundle to every node
that runs a worker and stops k0s from pulling images.

```
oms init k0s-config [flags]
```

### Examples

```
# Generate a k0sctl config into the OMS workdir
$ oms init k0s-config --install-config <path>

# Generate a k0sctl config at the given path
$ oms init k0s-config --install-config <path> --config <path>

# Generate an airgap k0sctl config using the airgap bundle from the OMS cache
$ oms init k0s-config --install-config <path> --airgapped

# Generate an airgap k0sctl config using the given airgap bundle
$ oms init k0s-config --install-config <path> --airgapped --path <bundle>

```

### Options

```
      --airgapped               Generate a config for installing k0s without internet access from the airgap image bundle
      --config string           Output file path for the k0sctl config (default: k0sctl-config-<datacenter>.yaml in the OMS workdir)
  -f, --force                   Force new downloads even if cached copies exist
  -h, --help                    help for k0s-config
      --install-config string   Path to Codesphere install-config file (required)
      --no-download             Skip downloading k0s binary (expects it to be on remote nodes)
  -p, --package string          Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from
      --path string             Path to the k0s airgap image bundle (requires --airgapped; default: the bundle in the OMS cache, downloaded if missing)
      --ssh-key-path string     SSH private key path for remote installation
  -v, --version string          Version of k0s to install (default "v1.31.14+k0s.0")
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms init](oms_init.md)	 - Initialize configuration files

