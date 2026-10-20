## oms cleanup k0s

Remove k0s from all nodes of a Codesphere install-config

### Synopsis

Remove k0s completely from all control plane and worker nodes of a Codesphere install-config.
Use this to recover from a failed or corrupted k0s installation without recreating the machines.

On every node this will:
- Stop and disable the k0sworker and k0scontroller services
- Run k0s reset
- Delete the k0s service units, /var/lib/k0s and /etc/k0s
- Reboot the node (skip with --skip-reboot)

The nodes are accessed as root via SSH.

```
oms cleanup k0s [flags]
```

### Examples

```
# Remove k0s from all nodes of the install-config
$ oms cleanup k0s --install-config <path>

# Use the given SSH private key to access the nodes
$ oms cleanup k0s --install-config <path> --ssh-key-path <path>

# Remove k0s without rebooting the nodes
$ oms cleanup k0s --install-config <path> --skip-reboot

# Skip the confirmation prompt
$ oms cleanup k0s --install-config <path> --force

```

### Options

```
  -f, --force                   Skip the confirmation prompt
  -h, --help                    help for k0s
      --install-config string   Path to Codesphere install-config file (required)
      --skip-reboot             Do not reboot the nodes after the cleanup
      --ssh-key-path string     SSH private key path to access the nodes
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms cleanup](oms_cleanup.md)	 - Remove installed components

