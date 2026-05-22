## oms beta install argocd-repo-secret

Create or update an ArgoCD repository secret

### Synopsis

Create or update an ArgoCD repository secret for authenticating against
Helm OCI registries or Git repositories.

The password is read from the OMS_REPO_PASSWORD environment variable.
If not set, it will be prompted interactively (hidden input).
You can also pipe the password via stdin: echo "token" | oms beta install argocd-repo-secret ...

```
oms beta install argocd-repo-secret [flags]
```

### Examples

```
# Create a Helm OCI registry secret (prompts for password)
$ oms install argocd-repo-secret --name ghcr-codesphere-helm-repo --url ghcr.io/codesphere-cloud/charts --repo-name ghcr-codesphere --type helm --username CodesphereBot --enable-oci

# Create a git repo credentials secret (set OMS_REPO_PASSWORD env var beforehand)
$ oms install argocd-repo-secret --name my-git-repo --url https://github.com/my-org --repo-name my-org --type git --username bot --secret-type repo-creds

```

### Options

```
      --enable-oci           Enable OCI support (sets enableOCI: "true" in the secret)
  -h, --help                 help for argocd-repo-secret
      --name string          Name of the Kubernetes Secret (metadata.name)
      --repo-name string     Display name for the repository in ArgoCD
      --secret-type string   ArgoCD secret type label value ("repository" or "repo-creds") (default "repository")
      --type string          Repository type: "helm" or "git"
      --url string           Repository URL (e.g. ghcr.io/codesphere-cloud/charts)
      --username string      Username for repository authentication
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

