## oms-cli beta argocd install

Install an ArgoCD helm release

### Synopsis

Install an ArgoCD helm release

```
oms-cli beta argocd install [flags]
```

### Examples

```
# Install an ArgoCD helm release of chart https://argoproj.github.io/argo-helm/argo-cd 
$ oms-cli install ArgoCD

# Version of the ArgoCD helm chart to install
$ oms-cli install ArgoCD --version <version>

```

### Options

```
      --dc-id string               Codesphere Datacenter ID where this ArgoCD is installed
  -c, --git-password string        Password/token to read from the git repo where ArgoCD Application manifests are stored
  -h, --help                       help for install
      --registry-password string   Password/token to read from the OCI registry (e.g. ghcr.io) where Helm chart artifacts are stored
  -v, --version string             Version of the ArgoCD helm chart to install
```

### SEE ALSO

* [oms-cli beta argocd](oms-cli_beta_argocd.md)	 - Commands to interact with ArgoCD

