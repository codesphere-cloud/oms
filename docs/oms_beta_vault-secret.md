## oms beta vault-secret

Create a Kubernetes secret from a vault file

### Synopsis

Create a Kubernetes secret from a prod.vault.yaml file.
Loads the selected vault type and creates a Kubernetes secret
with all the vault entries as key-value pairs in the target cluster.

```
oms beta vault-secret [flags]
```

### Examples

```
# Create secret using default age key location
$ oms vault-secret --vault-file prod.vault.yaml --namespace default --secret-name vault-secrets

# Create secret with explicit age key path
$ oms vault-secret --vault-file prod.vault.yaml --age-key /path/to/age_key.txt --namespace kube-system --secret-name cluster-secrets

```

### Options

```
      --age-key string       Path to the age key file (required for sops unless an age key environment variable is set)
  -h, --help                 help for vault-secret
      --namespace string     Kubernetes namespace where the secret will be created (default "codesphere")
      --secret-name string   Name of the Kubernetes secret to create (default "cs-vault")
      --vault-file string    Path to the vault file (required)
      --vault-type string    Vault storage type (sops or plain) (default "sops")
```

### SEE ALSO

* [oms beta](oms_beta.md)	 - Commands for early testing

