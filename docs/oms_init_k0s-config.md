## oms init k0s-config

Generate a k0sctl config for installing k0s

### Synopsis

Generate a k0sctl configuration without installing k0s. Nothing is
downloaded: the k0s binary is taken from the package or the OMS cache, see
'oms download k0s'.

The cluster is defined by a Codesphere install-config, an Ansible inventory and flags,
with increasing precedence: install-config < Ansible inventory < flags. Each source
only replaces the control planes or workers it defines, so --worker alone keeps the
control planes of the install-config or the Ansible inventory. Without an
install-config, --datacenter-name is required. A node given as both control plane and
worker runs both roles.

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

# Generate a k0sctl config for the Kubernetes hosts of an Ansible inventory
$ oms init k0s-config --ansible-inventory <path> --datacenter-name <name>

# Generate a k0sctl config for the given node IPs
$ oms init k0s-config --control-plane 10.0.0.1 --worker 10.0.0.2,10.0.0.3 --datacenter-name <name>

# Generate a k0sctl config from an install-config with the Kubernetes hosts of an Ansible inventory
$ oms init k0s-config --install-config <path> --ansible-inventory <path>

# Generate an airgap k0sctl config using the airgap bundle from the OMS cache
$ oms init k0s-config --install-config <path> --airgapped

# Generate an airgap k0sctl config using the given airgap bundle
$ oms init k0s-config --install-config <path> --airgapped --path <bundle>

```

### Options

```
      --airgapped                  Generate a config for installing k0s without internet access from the airgap image bundle
      --ansible-inventory string   Path to Ansible inventory file whose k8s-cp and k8s-workers hosts replace the ones of the install-config
      --api-server-host string     External host name or address of the Kubernetes API server, overriding the one of the install-config
      --control-plane strings      IP addresses of the Kubernetes control plane nodes, replacing the ones of the install-config and the Ansible inventory
      --datacenter-name string     Name of the data center, overriding the one of the install-config (required without --install-config)
  -f, --force                      Force extracting k0s from the package even if it was extracted before
  -h, --help                       help for k0s-config
      --install-config string      Path to Codesphere install-config file
      --no-download                Do not upload a k0s binary, the nodes download k0s themselves
  -o, --output string              Output file path for the k0sctl config (default: k0sctl-config-<datacenter>.yaml in the OMS workdir)
  -p, --package string             Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from
      --path string                Path to the k0s airgap image bundle (requires --airgapped; default: the bundle in the OMS cache)
      --ssh-key-path string        SSH private key path for remote installation
  -v, --version string             Version of k0s to install (default "v1.31.14+k0s.0")
      --worker strings             IP addresses of the Kubernetes worker nodes, replacing the ones of the install-config and the Ansible inventory
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms init](oms_init.md)	 - Initialize configuration files

