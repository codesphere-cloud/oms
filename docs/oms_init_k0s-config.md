## oms init k0s-config

Generate a k0sctl config for installing k0s

### Synopsis

Generate a k0sctl configuration from a Codesphere install-config without
installing k0s. Nothing is downloaded: the k0s binary is taken from the package or
the OMS cache, see 'oms download k0s'.

An existing k0sctl config is only overwritten after confirmation or with --yes.

With --airgapped, the generated config makes k0sctl upload the k0s airgap image
bundle to every node that runs a worker and stops k0s from pulling images.

```
oms init k0s-config [flags]
```

### Examples

```
# Generate a k0sctl config into the OMS workdir
$ oms init k0s-config --install-config <path>

# Generate a k0sctl config at the given path
$ oms init k0s-config --install-config <path> --output <path>

# Regenerate the k0sctl config, overwriting an existing one without asking
$ oms init k0s-config --install-config <path> --yes

# Generate an airgap k0sctl config using the airgap bundle from the OMS cache
$ oms init k0s-config --install-config <path> --airgapped

# Generate an airgap k0sctl config using the given airgap bundle
$ oms init k0s-config --install-config <path> --airgapped --path <bundle>

```

### Options

```
      --airgapped               Generate a config for installing k0s without internet access from the airgap image bundle
  -f, --force                   Force extracting k0s from the package even if it was extracted before
  -h, --help                    help for k0s-config
      --install-config string   Path to Codesphere install-config file (required)
      --no-download             Do not upload a k0s binary, the nodes download k0s themselves
  -o, --output string           Output file path for the k0sctl config (default: k0sctl-config-<datacenter>.yaml in the OMS workdir)
  -p, --package string          Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from
      --path string             Path to the k0s airgap image bundle (requires --airgapped; default: the bundle in the OMS cache)
      --ssh-key-path string     SSH private key path for remote installation
  -v, --version string          Version of k0s to install (default "v1.36.5+k0s.0")
  -y, --yes                     Overwrite an existing k0sctl config without asking
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms init](oms_init.md)	 - Initialize configuration files

