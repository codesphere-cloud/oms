## oms install openbao

Bootstrap OpenBao with Bank-Vaults Operator and DR backup

### Synopsis

Bootstrap OpenBao using the Bank-Vaults Operator with a KMS-less Day-0 workflow.

This command performs the full lifecycle:
1. Pre-flight DR check (restore from SOPS backup if exists)
2. Generate a secure password for userpass auth
3. Deploy the Bank-Vaults Operator via Helm
4. Apply the Vault CR with desired-state configuration
5. Wait for initialization to complete
6. Extract and encrypt unseal keys + password as SOPS DR backup

The command is idempotent and safe to re-run.

```
oms install openbao [flags]
```

### Examples

```
# Fresh bootstrap with DR backup saved locally
$ oms install openbao --dr-backup-path ./backups/cluster-1.enc.json

# Custom engine and user
$ oms install openbao --dr-backup-path ./backups/cluster-1.enc.json --secrets-engine my-engine --bao-user myuser

# Extended timeout for slower clusters
$ oms install openbao --dr-backup-path ./backups/cluster-1.enc.json --timeout 10m

```

### Options

```
  -k, --age-key-file string     Path to age private key file for SOPS encryption/decryption (auto-detected if not set)
      --bao-user string         Username for the userpass auth method (ignored on restore, uses DR backup value) (default "admin")
      --dr-backup-path string   Path for SOPS-encrypted DR backup file (required)
  -h, --help                    help for openbao
  -n, --namespace string        Kubernetes namespace for OpenBao deployment (default "vault")
      --replicas int            Number of OpenBao replicas (1 for single-node, odd number >= 3 for HA) (default 1)
      --secrets-engine string   Name of the KV-v2 secrets engine to provision (default "cs-secrets-engine")
      --storage-size string     PVC storage size for each OpenBao replica (default "10Gi")
      --timeout duration        Timeout for waiting on initialization (default 5m0s)
  -y, --yes                     Auto-approve re-initialization of an existing deployment when no DR backup is found
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components

