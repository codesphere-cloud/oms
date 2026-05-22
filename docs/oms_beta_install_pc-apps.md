## oms beta install pc-apps

Install the pc-apps Helm chart from a private OCI registry

### Synopsis

Install or upgrade the pc-apps Helm chart from a private OCI registry
into the target cluster. This chart deploys ArgoCD Application resources
that manage the platform components.

The registry password is read from the OMS_REPO_PASSWORD environment variable.
If not set, it will be prompted interactively (hidden input).

```
oms beta install pc-apps [flags]
```

### Examples

```
# Install a specific version (prompts for password)
$ oms install pc-apps --chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --version 1.0.0 --username CodesphereBot

# Install latest with multiple values files
$ oms install pc-apps --chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --username CodesphereBot -f base.yaml -f dc-overlay.yaml

# Install into a custom namespace
$ oms install pc-apps --chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --username CodesphereBot --namespace custom-ns

```

### Options

```
      --chart string         Full OCI chart URL (e.g. oci://ghcr.io/codesphere-cloud/charts/pc-apps)
  -h, --help                 help for pc-apps
      --namespace string     Target namespace for the Helm release (default "argocd")
      --username string      Username for OCI registry authentication
  -f, --values stringArray   Path to values YAML file (can be specified multiple times, merged in order)
      --version string       Chart version to install (default: latest)
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

