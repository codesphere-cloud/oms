## oms install codesphere dependencies

Install Codesphere cluster dependencies (Phase 2)

### Synopsis

Install cluster dependencies for a Codesphere instance (Phase 2).
Runs ArgoCD install, vault secret sync, and pc-apps deployment first, then steps: set-up-cluster, ms-backends.
Requires the infrastructure phase to have completed successfully.
Pass --skip-steps argocd or add argocd to operations.skip to skip the ArgoCD pre-step.

```
oms install codesphere dependencies [flags]
```

### Examples

```
# Install cluster dependencies (including ArgoCD)
$ oms install codesphere dependencies -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml

# Install cluster dependencies without the ArgoCD pre-step
$ oms install codesphere dependencies -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml -s argocd

# Install cluster dependencies with custom pc-apps values files
$ oms install codesphere dependencies -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml --pc-apps-values base.yaml --pc-apps-values dc-overlay.yaml

```

### Options

```
  -h, --help   help for dependencies
```

### Options inherited from parent commands

```
      --argo-force-conflicts         Force SSA ownership conflicts during ArgoCD install
      --argo-registry-url string     OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)
      --argo-repo string             ArgoCD Helm chart repository URL (default "https://argoproj.github.io/argo-helm")
      --argo-values stringArray      ArgoCD values YAML file (can be specified multiple times)
      --argo-version string          ArgoCD Helm chart version to install
      --auto-approve                 Auto approve confirmation prompts with default values (default true)
  -c, --config stringArray           Path to a Codesphere Private Cloud configuration file (yaml). Can be specified multiple times and merged in order
      --direct-connection            Use direct connection for installation, requires having access to the cluster nodes from your machine
  -f, --force                        Enforce package extraction
  -p, --package string               Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
      --pc-apps-values stringArray   pc-apps values YAML file (can be specified multiple times)
  -k, --priv-key string              Path to the private key to encrypt/decrypt secrets
  -s, --skip-steps strings           Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd
      --vault string                 Path to the SOPS-encrypted prod.vault.yaml file used for config templating
```

### SEE ALSO

* [oms install codesphere](oms_install_codesphere.md)	 - Install a Codesphere instance

