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
$ oms install codesphere infra -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml

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
      --argo-registry-ca string      Path to the certificate authority of the OCI registry, needed when its certificate is self-signed
      --argo-registry-url string     OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)
      --argo-repo string             Upstream ArgoCD chart repository; requires --argo-version (default "https://argoproj.github.io/argo-helm")
      --argo-values stringArray      ArgoCD values YAML file; packaged chart settings go under argo-cd (repeatable)
      --argo-version string          Upstream ArgoCD Helm chart version; overrides the packaged chart
      --auto-approve                 Auto approve confirmation prompts with default values (default true)
  -c, --config stringArray           Path to a Codesphere Private Cloud configuration file (yaml). Can be specified multiple times and merged in order
      --direct-connection            Use direct connection for installation, requires having access to the cluster nodes from your machine
  -f, --force                        Enforce package extraction
      --local-components             Run selected install components on the local host
      --local-config-dir string      Config directory for local install components
  -p, --package string               Package file to use. This flag overrides the version specified in the configuration file.
      --pc-apps-values stringArray   pc-apps values YAML file (can be specified multiple times)
  -k, --priv-key string              Path to the age private key (required for sops unless an age key environment variable is set)
  -s, --skip-steps strings           Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd
      --timer                        Show how long each step took and a final timing summary table
      --vault string                 Path to the prod.vault.yaml file used for config templating
      --vault-secret-name string     Secret in the codesphere namespace the vault is written to (as source for ESO). (default "cs-vault")
      --verbose                      Enable verbose output
```

### SEE ALSO

* [oms install codesphere](oms_install_codesphere.md)	 - Install a Codesphere instance

