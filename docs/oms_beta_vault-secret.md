## oms beta vault-secret

Create a Kubernetes secret from a SOPS-encrypted vault file

### Synopsis

Create a Kubernetes secret from a SOPS-encrypted prod.vault.yaml file.
Reads the encrypted vault file, decrypts it using the age key, and creates a Kubernetes secret
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
      --age-key string       Path to the age key file (optional, will use defaults if not provided)
  -h, --help                 help for vault-secret
      --namespace string     Kubernetes namespace where the secret will be created (default "codesphere")
      --secret-name string   Name of the Kubernetes secret to create (default "cs-vault")
      --vault-file string    Path to the SOPS-encrypted vault file (required)
```

### SEE ALSO

* [oms beta](oms_beta.md)	 - Commands for early testing

