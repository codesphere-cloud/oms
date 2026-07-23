## oms beta bootstrap-local

Bootstrap a local Codesphere environment

### Synopsis

Bootstraps a local Codesphere environment using a single Linux x86_64 Kubernetes cluster.
Rook is used to install Ceph, and CNPG is used for the PostgreSQL database.
For local setups, use Minikube with a virtual machine on Linux.
Not for production use.

```
oms beta bootstrap-local [flags]
```

### Options

```
      --argocd                       After infra setup: install ArgoCD, update the OCI pull secret, and install pc-apps from the BOM version (default true)
      --base-domain string           Base domain for Codesphere (default "cs.local")
      --feature-flags stringArray    Feature flags to enable in Codesphere installation (optional)
  -h, --help                         help for bootstrap-local
      --install-config string        Path to install config file (default: <install-dir>/config.yaml)
      --install-dir string           Directory for config, secrets, and bundle files (default ".installer")
      --install-hash string          Codesphere package hash (required when install-version is set)
      --install-local string         Path to a local installer package (tar.gz or unpacked directory)
      --install-version string       Codesphere version to install (downloaded from the OMS portal)
      --internal-flags stringArray   Internal flags to enable in Codesphere installation (optional) (default [headless-services,vcluster,custom-service-image,ms-in-ls])
      --k0s                          Use k0s-specific configuration (required to deploy to k0s clusters)
      --pod-cidr string              Service CIDR of the Kubernetes cluster. If not specified, OMS will try to determine it.
      --preview-flags stringArray    Preview flags to enable in Codesphere installation (optional) (default [secret-management,sub-path-mount,workspace-ssh])
      --profile string               Profile to apply to the install config like resources (supported: dev, minimal, prod) (default "dev")
      --registry-url string          OCI registry URL used for the ArgoCD helm pull secret (only relevant with --argocd) (default "oci://ghcr.io/codesphere-cloud/charts")
      --registry-user string         Custom Registry username (optional)
      --secrets-file string          Path to secrets file (default: <install-dir>/prod.vault.yaml)
      --service-cidr string          Service CIDR of the Kubernetes cluster. If not specified, OMS will try to determine it.
  -y, --yes                          Auto-approve the local bootstrapping warning prompt
```

### SEE ALSO

* [oms beta](oms_beta.md)	 - Commands for early testing

