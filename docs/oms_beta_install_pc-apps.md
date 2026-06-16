## oms beta install pc-apps

Install the pc-applications Helm chart from a private OCI registry

### Synopsis

Install or upgrade the pc-applications Helm chart from a private OCI
registry into the target cluster. This chart deploys ArgoCD Application
resources that manage the platform components.

Registry credentials and chart URL are read automatically from the
Kubernetes secret "argocd-codesphere-oci-read" in the argocd namespace.
This secret is created by "oms beta install argocd --deploy-dc-config".

```
oms beta install pc-apps [flags]
```

### Examples

```
# Install a specific version
$ oms beta install pc-apps --version 1.0.0

# Install with custom values files
$ oms beta install pc-apps --version 1.0.0 -f base.yaml -f dc-overlay.yaml

# Install into a custom namespace
$ oms beta install pc-apps --version 1.0.0 --namespace custom-ns

# Force SSA ownership conflicts during install or upgrade
$ oms beta install pc-apps --version 1.0.0 --force-conflicts

```

### Options

```
      --force-conflicts      Force field ownership conflicts during install or upgrade (sets server-side apply ForceConflicts)
  -h, --help                 help for pc-apps
      --namespace string     Target namespace for the Helm release (default "argocd")
  -f, --values stringArray   Path to values YAML file (can be specified multiple times, merged in order)
      --version string       Chart version to install (required)
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

