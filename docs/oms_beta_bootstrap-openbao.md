## oms beta bootstrap-openbao

Export or restore the unseal keys of the ArgoCD-deployed OpenBao

### Synopsis

Handle the OpenBao steps ArgoCD cannot do for the instance deployed by the openbao chart.

By default, waits for the bank-vaults operator to initialize the instance and exports
the openbao-unseal-keys Secret to a SOPS-encrypted DR backup. Sync the OpenBao ArgoCD
app first; the command fails if the Vault CR does not exist yet.

With --restore, seeds openbao-unseal-keys from the DR backup so bank-vaults unseals
with the original keys instead of re-initializing. Run it before the first sync of the
OpenBao ArgoCD app, then sync while the command waits for the instance to become ready.

```
oms beta bootstrap-openbao [flags]
```

### Examples

```
# Export the unseal keys to a new DR backup
$ oms beta bootstrap-openbao --dr-backup-path ./backups/cluster-1.enc.json

# Restore the unseal keys from an existing DR backup
$ oms beta bootstrap-openbao --dr-backup-path ./backups/cluster-1.enc.json --restore

# Extended timeout for slower clusters
$ oms beta bootstrap-openbao --dr-backup-path ./backups/cluster-1.enc.json --timeout 10m

```

### Options

```
  -k, --age-key-file string     Path to age private key file for SOPS encryption/decryption (auto-detected if not set)
      --dr-backup-path string   Path of the SOPS-encrypted DR backup file (required)
  -h, --help                    help for bootstrap-openbao
  -n, --namespace string        Kubernetes namespace of the OpenBao deployment (default "vault")
      --restore                 Seed the unseal keys from the DR backup instead of exporting them
      --timeout duration        Timeout for waiting on the Vault CR and initialization; pod readiness additionally allows 3m per replica (default 5m0s)
      --timer                   Show how long each step took after it finishes
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms beta](oms_beta.md)	 - Commands for early testing

