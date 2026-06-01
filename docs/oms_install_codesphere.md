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
      --age-key string          Path to the age private key used to decrypt --vault-file (optional, uses default search paths if omitted)
      --argocd                  After installation: deploy vault secrets, update the ArgoCD OCI pull secret, and install pc-apps from the BOM version
      --codesphere-only         Install only Codesphere without dependencies
  -c, --config string           Path to the Codesphere Private Cloud configuration file (yaml)
      --direct-connection       Use direct connection for installation, requires having access to the cluster nodes from your machine
  -f, --force                   Enforce package extraction
  -h, --help                    help for codesphere
  -p, --package string          Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from
  -k, --priv-key string         Path to the private key to encrypt/decrypt secrets
      --registry-url string     OCI registry URL used for the ArgoCD helm pull secret (only relevant with --argocd) (default "ghcr.io/codesphere-cloud/charts")
  -s, --skip-steps strings      Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, kubernetes
      --vault string            Path to the SOPS-encrypted prod.vault.yaml file used for config templating (default "prod.vault.yaml")
      --vault-file string       Path to the SOPS-encrypted vault file to deploy as a Kubernetes secret (only relevant with --argocd)
      --vault-namespace string  Kubernetes namespace for the vault secret (only relevant with --argocd) (default "codesphere")
      --vault-secret-name string Name of the Kubernetes secret created from the vault (only relevant with --argocd) (default "cs-vault")
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components
