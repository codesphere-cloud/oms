## oms install codesphere

Install a Codesphere instance

### Synopsis

Install a Codesphere instance with the provided configuration file and private key.
When --package is omitted, operations.codesphere selects the version to download
and reuse in OMS_WORKDIR. An explicit --package overrides the configured version.
Uses the private-cloud-installer.js script included in the package to perform the installation.
Use the dependencies and platform subcommands with --local-components for a prepared local cluster.

```
oms install codesphere [flags]
```

### Examples

```
# Install the operations.codesphere version, downloading it to OMS_WORKDIR if needed
$ oms install codesphere -c config.yaml -k <path-to-private-key>

# Skip most pre-installation steps. E.g. if you only need to re-apply Codesphere's helm charts
$ oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s copy-dependencies,extract-dependencies,load-container-images,ceph,postgres,kubernetes,docker

# Skip loading container images. Necessary when installing a lite package that doesn't include any container images
$ oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images

```

### Options

```
      --argo-force-conflicts         Force SSA ownership conflicts during ArgoCD install
      --argo-registry-ca string      Path to the certificate authority of the OCI registry, needed when its certificate is self-signed
      --argo-registry-url string     OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)
      --argo-repo string             ArgoCD Helm chart repository URL (default "https://argoproj.github.io/argo-helm")
      --argo-values stringArray      ArgoCD values YAML file (can be specified multiple times)
      --argo-version string          ArgoCD Helm chart version to install
      --auto-approve                 Auto approve confirmation prompts with default values (default true)
      --codesphere-only              Install only Codesphere without dependencies
  -c, --config stringArray           Path to a Codesphere Private Cloud configuration file (yaml). Can be specified multiple times and merged in order
      --direct-connection            Use direct connection for installation, requires having access to the cluster nodes from your machine
      --extra-vault-secret string    Also write the vault into this Secret in the codesphere namespace before deploying Codesphere, e.g. for preview releases sharing a cluster
  -f, --force                        Enforce package extraction
  -h, --help                         help for codesphere
      --local-components             Run selected install components on the local host
      --local-config-dir string      Config directory for local install components
  -p, --package string               Package file to use. This flag overrides the version specified in the configuration file.
      --pc-apps-values stringArray   pc-apps values YAML file (can be specified multiple times)
  -k, --priv-key string              Path to the age private key (required for sops unless an age key environment variable is set)
  -s, --skip-steps strings           Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd
      --timer                        Show how long each step took after it finishes
      --vault string                 Path to the prod.vault.yaml file used for config templating
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components
* [oms install codesphere dependencies](oms_install_codesphere_dependencies.md)	 - Install Codesphere cluster dependencies (Phase 2)
* [oms install codesphere infra](oms_install_codesphere_infra.md)	 - Install Codesphere infrastructure (Phase 1)
* [oms install codesphere platform](oms_install_codesphere_platform.md)	 - Install the Codesphere platform (Phase 3)

