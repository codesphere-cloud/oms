## oms beta install pc-apps

Install the pc-apps Helm chart from a private OCI registry

### Synopsis

Install or upgrade the pc-apps Helm chart from a private OCI registry
into the target cluster. This chart deploys ArgoCD Application resources
that manage the platform components.

If --username is provided, the registry password is read from the
OMS_REPO_PASSWORD environment variable or prompted interactively.
Otherwise, credentials are read from the Kubernetes secret
"argocd-codesphere-oci-read" in the argocd namespace (created by
"oms beta install argocd --deploy-dc-config --registry-password <token>").

```
oms beta install pc-apps [flags]
```

### Examples

```
# Install a specific version (credentials from K8s secret)
$ oms beta install pc-apps --version 1.0.0

# Install with explicit registry credentials (prompts for password)
$ oms beta install pc-apps --version 1.0.0 --username CodesphereBot

# Install with custom chart and values files
$ oms beta install pc-apps --chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --version 1.0.0 -f base.yaml -f dc-overlay.yaml

```

### Options

```
      --chart string         Full OCI chart URL (default "oci://ghcr.io/codesphere-cloud/charts/pc-apps")
  -h, --help                 help for pc-apps
      --namespace string     Target namespace for the Helm release (default "argocd")
      --username string      Username for OCI registry authentication (if omitted, reads from K8s secret)
  -f, --values stringArray   Path to values YAML file (can be specified multiple times, merged in order)
      --version string       Chart version to install (default: latest)
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

