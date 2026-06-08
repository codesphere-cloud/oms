## oms beta install argocd

Install an ArgoCD helm release

### Synopsis

Install or upgrade the ArgoCD helm release.

When --deploy-dc-config is set, Codesphere-managed resources are applied after
the chart install/upgrade:
  - AppProjects (always)
  - Helm OCI registry secret (always, requires OMS_REGISTRY_PASSWORD)
  - Local cluster secret (only if --dc-id is provided)
  - Git repo credentials (only if OMS_GIT_PASSWORD env var is set)

Use --registry-url to point to a custom or mirrored OCI registry (defaults
to ghcr.io/codesphere-cloud/charts).

Environment variables:
  OMS_REGISTRY_PASSWORD  Password/token for the Helm OCI registry (required for --deploy-dc-config)
  OMS_GIT_PASSWORD       Password/token for git repo access (optional)

```
oms beta install argocd [flags]
```

### Examples

```
# Install ArgoCD helm chart only
$ oms beta install argocd

# Install a specific chart version
$ oms beta install argocd --version 7.8.0

# Install chart and apply Codesphere resources (prompts for OCI password)
$ oms beta install argocd --deploy-dc-config

# Also register the local cluster as dc-0
$ oms beta install argocd --deploy-dc-config --dc-id 0

```

### Options

```
      --dc-id string          Codesphere Datacenter ID (optional, registers local cluster in ArgoCD)
      --deploy-dc-config      Apply Codesphere-managed resources (AppProjects, Repo Creds, ...) after installing the chart
      --force-conflicts       Force field ownership conflicts during upgrade (sets server-side apply ForceConflicts)
  -h, --help                  help for argocd
      --registry-url string   OCI registry URL for the Helm chart repository (default "ghcr.io/codesphere-cloud/charts")
      --repo string           Helm chart repository URL; supports HTTP (default: https://argoproj.github.io/argo-helm) and OCI (e.g. oci://ghcr.io/argoproj/argo-helm)
  -f, --values stringArray    Specify values in a YAML file (can be specified multiple times)
  -v, --version string        Version of the ArgoCD helm chart to install
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

