## oms beta install argocd-repo-secret

Create or update the Codesphere Helm repository secret in ArgoCD

### Synopsis

Create or update the ArgoCD repository secret for authenticating against
the Codesphere Helm chart OCI registry.

Use --url to point to a mirror of the registry if needed.

The password is read from the OMS_REPO_PASSWORD environment variable.
If not set, it will be prompted interactively (hidden input).
You can also pipe the password via stdin: echo "token" | oms beta install argocd-repo-secret ...

```
oms beta install argocd-repo-secret [flags]
```

### Examples

```
# Create the secret using defaults (prompts for password)
$ oms beta install argocd-repo-secret

# Use a mirrored registry URL
$ oms beta install argocd-repo-secret --url my-mirror.example.com/charts

# Use a mirrored registry with custom username
$ oms beta install argocd-repo-secret --url my-mirror.example.com/charts --username MyBot

```

### Options

```
  -h, --help              help for argocd-repo-secret
      --url string        Helm OCI registry URL (customize for mirrors) (default "ghcr.io/codesphere-cloud/charts")
      --username string   Username for registry authentication (default "CodesphereBot")
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

