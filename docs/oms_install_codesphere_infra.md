## oms install codesphere infra

Install Codesphere infrastructure (Phase 1)

### Synopsis

Install infrastructure dependencies for a Codesphere instance (Phase 1).
Runs steps: copy-dependencies, extract-dependencies, load-container-images, sops, docker, postgres, ceph, kubernetes.

```
oms install codesphere infra [flags]
```

### Examples

```
# Install infrastructure components only
$ oms install codesphere infra -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml

# Skip loading container images when using a lite package
$ oms install codesphere infra -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images

```

### Options

```
  -h, --help   help for infra
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

