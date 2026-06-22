## oms install codesphere

Install a Codesphere instance

### Synopsis

Install a Codesphere instance with the provided package, configuration file, and private key.
Uses the private-cloud-installer.js script included in the package to perform the installation.

```
oms install codesphere [flags]
```

### Examples

```
# Skip most pre-installation steps. E.g. if you only need to re-apply Codesphere's helm charts
$ oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s copy-dependencies,extract-dependencies,load-container-images,ceph,postgres,kubernetes,docker

# Skip loading container images. Necessary when installing a lite package that doesn't include any container images
$ oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images

```

### Options

```
      --argo-force-conflicts         Force SSA ownership conflicts during ArgoCD install
      --argo-registry-url string     OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)
      --argo-repo string             ArgoCD Helm chart repository URL (default "https://argoproj.github.io/argo-helm")
      --argo-values stringArray      ArgoCD values YAML file (can be specified multiple times)
      --argo-version string          ArgoCD Helm chart version to install
      --auto-approve                 Auto approve confirmation prompts with default values (default true)
      --codesphere-only              Install only Codesphere without dependencies
  -c, --config stringArray           Path to a Codesphere Private Cloud configuration file (yaml). Can be specified multiple times and merged in order
      --direct-connection            Use direct connection for installation, requires having access to the cluster nodes from your machine
  -f, --force                        Enforce package extraction
  -h, --help                         help for codesphere
  -p, --package string               Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
      --pc-apps-values stringArray   pc-apps values YAML file (can be specified multiple times)
  -k, --priv-key string              Path to the private key to encrypt/decrypt secrets
  -s, --skip-steps strings           Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd
      --vault string                 Path to the SOPS-encrypted prod.vault.yaml file used for config templating
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components
* [oms install codesphere dependencies](oms_install_codesphere_dependencies.md)	 - Install Codesphere cluster dependencies (Phase 2)
* [oms install codesphere infra](oms_install_codesphere_infra.md)	 - Install Codesphere infrastructure (Phase 1)
* [oms install codesphere platform](oms_install_codesphere_platform.md)	 - Install the Codesphere platform (Phase 3)

