[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![Build Status](https://github.com/codesphere-cloud/oms/actions/workflows/cli-build_test.yml/badge.svg)
![Integration Test Status](https://github.com/codesphere-cloud/oms/actions/workflows/integration-test.yml/badge.svg)

# Operations Management System - OMS

This repository contains the source for the operations management system. It
contains the sources for both the CLI and the Service. 

## OMS CLI

The OMS CLI tool is used to bootstrap Codesphere cluster on customer sites and
replaces the formerly used private cloud installer.

### Installation

You can install the OMS CLI in a few ways:

#### Using GitHub CLI (`gh`)

If you have the [GitHub CLI](https://cli.github.com/) installed, you can install the OMS CLI with a command like the following.
Note that some commands may require you to elevate to the root user with `sudo`.

##### ARM Mac

```
gh release download -R codesphere-cloud/oms -O /usr/local/bin/oms -p "oms*darwin_arm64"
chmod +x /usr/local/bin/oms
```

##### Linux Amd64

```
gh release download -R codesphere-cloud/oms -O /usr/local/bin/oms -p "oms*linux_amd64"
chmod +x /usr/local/bin/oms
```

#### Using `wget`

This option requires to have the `wget` and `jq` utils installed. Download the OMS CLI and add permissions to run it with the following commands:
Note that some commands may require you to elevate to the root user with `sudo`.

##### ARM Mac

```
wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match("oms.*darwin_arm64")) | .browser_download_url' | xargs wget -O oms
mv oms /usr/local/bin/oms
chmod +x /usr/local/bin/oms
```

##### Linux Amd64

```
wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match("oms.*linux_amd64")) | .browser_download_url' | xargs wget -O oms
mv oms /usr/local/bin/oms
chmod +x /usr/local/bin/oms
```

#### Manual Download

You can also download the pre-compiled binaries from the [OMS Releases page](https://github.com/codesphere-cloud/oms/releases).
Note that some commands may require you to elevate to the root user with `sudo`.

1. Go to the [latest release](https://github.com/codesphere-cloud/oms/releases/latest).

2. Download the appropriate release for your operating system and architecture (e.g., `oms_darwin_amd64` for macOS, `oms_linux_amd64` for Linux, or `oms_windows_amd64` for Windows).

3. Move the `oms` binary to a directory in your system's `PATH` (e.g., `/usr/local/bin` on Linux/Mac, or a directory added to `Path` environment variable on Windows).

4. Make the binary executable (e.g. by running `chmod +x /usr/local/bin/oms` on Mac or Linux)

#### Available Commands

The OMS CLI organizes its functionality into several top-level commands, each with specific subcommands and flags.

See our [Usage Documentation](docs) for usage information about the specific subcommands.

##### `oms install codesphere`

Install a Codesphere instance with the provided package, configuration file, and private key.
Uses the `private-cloud-installer.js` script included in the package to perform the installation.

```
oms install codesphere [flags]
```

**Examples**

```sh
# Skip most pre-installation steps (e.g. re-apply Codesphere helm charts only)
oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml \
  -s copy-dependencies,extract-dependencies,load-container-images,ceph,postgres,kubernetes,docker

# Skip loading container images (lite package without images)
oms install codesphere -p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml \
  -s load-container-images

# Full install with ArgoCD-based post-steps: deploy vault secrets, update OCI pull secret, install pc-apps
oms install codesphere -p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml \
  --argocd --vault-file prod.vault.yaml --age-key age_key.txt
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `-p, --package` | | Package file (`.tar.gz`) to load binaries and installer from (**required**) |
| `-c, --config` | | Path to the Codesphere Private Cloud configuration file (yaml) (**required**) |
| `-k, --priv-key` | | Path to the private key to encrypt/decrypt secrets (**required**) |
| `-f, --force` | `false` | Enforce package extraction |
| `-s, --skip-steps` | | Steps to skip (e.g. `copy-dependencies`, `load-container-images`, `ceph`, `kubernetes`) |
| `--codesphere-only` | `false` | Install only Codesphere without dependencies |
| `--direct-connection` | `false` | Use direct connection to cluster nodes |
| `--argocd` | `false` | After installation: deploy vault secrets, update ArgoCD OCI pull secret, and install pc-apps from the BOM version |
| `--vault-file` | | Path to the SOPS-encrypted vault file to deploy as a Kubernetes secret (with `--argocd`) |
| `--age-key` | | Path to the age private key for vault decryption (with `--argocd`, optional) |
| `--vault-namespace` | `codesphere` | Kubernetes namespace for the vault secret (with `--argocd`) |
| `--vault-secret-name` | `cs-vault` | Name of the Kubernetes secret created from the vault (with `--argocd`) |
| `--registry-url` | `ghcr.io/codesphere-cloud/charts` | OCI registry URL for the ArgoCD helm pull secret (with `--argocd`) |

**ArgoCD integration (`--argocd`)**

When `--argocd` is set the following steps run after the node installer completes:

1. **Vault secrets** — decrypts `--vault-file` with the age key and creates/updates the K8s secret `--vault-secret-name` in `--vault-namespace`. Skipped if `--vault-file` is not provided.
2. **ArgoCD OCI pull secret** — creates or updates the `argocd-codesphere-oci-read` secret in the `argocd` namespace. Requires `OMS_REGISTRY_PASSWORD` to be set or an interactive terminal for the prompt.
3. **pc-applications** — reads the chart version from `bom.json` (`components["pc-applications"].files.chart.ociRef`) and installs or upgrades the `pc-applications` Helm chart. Prints a warning and skips if the component is absent from the BOM.

### How to Build?

```shell
make build-cli
```

See also [CONTRIBUTION.md]

## Service

The service implementation is currently WIP

### How to Build?

```shell
make build-service
```

## Community & Contributions

Please review our [Code of Conduct](CODE_OF_CONDUCT.md) to understand our community expectations.
We welcome contributions! All contributions to this project must be made in accordance with the Developer Certificate of Origin (DCO). See our full [Contributing Guidelines](CONTRIBUTING.md) for details.
