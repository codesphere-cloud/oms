## oms beta install pc-apps

Register the pc-applications app-of-apps in ArgoCD

### Synopsis

Create or update the "pc-applications" ArgoCD Application (app of apps)
that references the pc-applications Helm chart in a private OCI registry.
ArgoCD pulls and syncs the chart itself, which in turn deploys the
ArgoCD Application resources managing the platform components.

The chart registry URL is read from the Kubernetes secret
"argocd-codesphere-oci-read" in the argocd namespace, which also provides
ArgoCD with the credentials to pull the chart. This secret is created by
"oms beta install argocd --deploy-dc-config".

```
oms beta install pc-apps [flags]
```

### Examples

```
# Register a specific chart version
$ oms beta install pc-apps --version 1.0.0

# Register with custom values files
$ oms beta install pc-apps --version 1.0.0 -f base.yaml -f dc-overlay.yaml

# Deploy the apps into a custom namespace
$ oms beta install pc-apps --version 1.0.0 --namespace custom-ns

# Force SSA ownership conflicts when applying the Application
$ oms beta install pc-apps --version 1.0.0 --force-conflicts

```

### Options

```
      --force-conflicts      Force field ownership conflicts when applying the Application (sets server-side apply ForceConflicts)
  -h, --help                 help for pc-apps
      --namespace string     Destination namespace the app-of-apps deploys into (default "argocd")
  -f, --values stringArray   Path to values YAML file (can be specified multiple times, merged in order)
      --version string       Chart version to reference as the Application target revision (required)
```

### SEE ALSO

* [oms beta install](oms_beta_install.md)	 - Install beta components

