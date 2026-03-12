## oms beta install argocd

Install an ArgoCD helm release

### Synopsis

Install an ArgoCD helm release

```
oms beta install argocd [flags]
```

### Examples

```
# Install an ArgoCD helm release of chart https://argoproj.github.io/argo-helm/argo-cd 
$ oms install ArgoCD

# Version of the ArgoCD helm chart to install
$ oms install ArgoCD --version <version>

```

### Options

```
      --dc-id string               Codesphere Datacenter ID where this ArgoCD is installed
      --full-install               Install other resources (AppProjects, Repo Creds, ...) after installing the chart
      --git-password string        Password/token to read from the git repo where ArgoCD Application manifests are stored
  -h, --help                       help for argocd
      --registry-password string   Password/token to read from the OCI registry (e.g. ghcr.io) where Helm chart artifacts are stored
  -v, --version string             Version of the ArgoCD helm chart to install
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

