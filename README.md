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

For the complete infrastructure preparation and installation workflow, see [Prepare infrastructure and install Codesphere](docs/install-codesphere-on-any-infrastructure.md).

Use `oms beta bootstrap-gcp` or `oms beta bootstrap-gcp multi-node` for a multi-node GCP cluster; the bare command keeps its existing multi-node behavior. For an all-in-one GCP test installation, `oms beta bootstrap-gcp single-node` creates one VM with an extra Ceph disk, runs `oms install k0s --single`, and runs `bootstrap-local --expose-shared` on the VM using the installer archive. Both bootstrap modes accept `--install-config-template` as an initial overlay and repeated `--config` files merged in order before infrastructure settings; use `--vault` and `--priv-key` when they reference SOPS vault secrets. The command uploads the config files and applies VM-specific settings to the result. `--install-config-template` remains available for one initial base file. The local bootstrap runs `oms install codesphere dependencies` and then `oms install codesphere platform`; the dependencies phase installs Argo CD. It writes the effective config to `single-vm-config.yaml` in the OMS workdir, creates Google Public CA EAB credentials in `single-vm-vault.yaml`, and provides both files to the VM. The VM's one external IP serves HTTPS on port 443, HTTP on port 80, and the SSH workspace proxy on port 2222; VM administration stays on port 22. Set `OMS_REGISTRY_PASSWORD` and provide `--project-name`, `--billing-account`, `--base-domain`, `--registry-user`, and either `--install-local` for an archive on your machine or `--install-version` to download one through the OMS portal. The boot disk defaults to 50 GB in both modes; use `--root-disk-size` to override it. The extra 100 GB Ceph disk uses the fixed Rook device filter `^sdb$`. Both modes reuse OMS on the remote host or download the latest release when absent; optionally supply `--remote-oms-binary` to upload a local Linux amd64 build. Use `--machine-type` to override the machine type for every VM in either mode; omitting it preserves the existing sizes for each role. Use `--os rhel-9` to boot RHEL 9.6 with Extended Update Support instead of the default Ubuntu 22.04; GCP bills this as a premium image and VMs that already exist keep their OS. It works together with `--airgapped`; see [Supported Host Operating Systems](#supported-host-operating-systems). The GCP account must be allowed to create projects, VMs, service accounts, DNS records, and Public CA external account keys. The command writes the usual `gcp-infra.json`, and `oms beta bootstrap-gcp cleanup` removes the project and DNS records. This workflow is for testing. VMs use standard provisioning by default; pass `--spot-vms` to use a Spot VM, which can be interrupted and falls back to a standard VM if Spot capacity is unavailable.

With `--airgapped`, the connected jumpbox stages cloud-controller manifests and images immediately after k0s installation, before Codesphere is installed. Nodes keep internet egress blocked; Private Google Access and a VPC-private DNS zone route Google API requests to `199.36.153.8/30`, with HTTPS allowed for k0s nodes so the cloud controller can manage load balancers. Reserved IPs are configured when the gateway Services are created.

The jumpbox issues DNS-01 certificates using its Cloud DNS service account for `cs.<base-domain>`, the workspace domain and their wildcards, plus per-data-center platform names in multi-DC installations. ACME accounts and certificates are cached in `/root/oms-acme`; the local bootstrap process downloads and validates the certificate and key, updates its install config and vault, then uploads them over SSH and encrypts the vault on the jumpbox. The platform and public-gateway charts create the TLS Secrets without cluster-side ACME requests. `--acme-staging` and `--google-acme-issuer` select the same ACME servers as connected installs. Renewals are not scheduled; rerunning bootstrap reuses a valid cached certificate or obtains a replacement. Staging certificates are not browser-trusted.

#### Supported Host Operating Systems

Codesphere hosts installed with OMS run Ubuntu 22.04 or RHEL 9.6 (x86-64). Other RHEL releases and RHEL derivatives such as Rocky Linux or AlmaLinux are not supported.

`oms beta bootstrap-gcp` boots Ubuntu 22.04 by default. Pass `--os rhel-9` to `oms beta bootstrap-gcp`, `multi-node`, or `single-node` to boot every VM from the `rhel-9-6-eus` image family instead. The Extended Update Support (EUS) repositories keep the hosts on RHEL 9.6, and GCP bills the image as a premium image. The OS is applied when a VM is created, so a re-run with a different `--os` does not reinstall VMs that already exist.

On RHEL, OMS prepares the hosts as follows:

- Every node gets `lvm2`, which Ceph needs to create its OSDs, and `container-selinux`, which the Docker packages require.
- Every node gets Docker CE from Docker's RHEL 9 repository, which OMS writes to `/etc/yum.repos.d/docker-ce.repo`. The `podman-docker` package is removed first because it conflicts with `docker-ce`. Nodes that already run Docker CE are left untouched. The installer's `docker` step only checks that Docker is running, so Docker must be present before the installation starts.
- The jumpbox registry runs with `podman` and `httpd-tools`, trusts its certificate through `update-ca-trust`, and mounts its files with `:Z` so that SELinux permits access.
- SELinux stays in enforcing mode. OMS does not change it.

##### Air-gapped installations

With `--airgapped`, only the jumpbox can reach the internet. OMS runs a Squid package proxy as a container on the jumpbox (port 3128, reachable from the VPC only) and points the package manager of every node at it: `proxy=` in `/etc/dnf/dnf.conf` on RHEL, and `/etc/apt/apt.conf.d/99oms-package-proxy` on Ubuntu. The nodes stay without any other outbound connectivity.

The proxy only allows the hosts of the package repositories configured on the jumpbox, plus `download.docker.com` on RHEL. The jumpbox and the nodes must therefore boot the same OS image, which the bootstrap guarantees because `--os` applies to all VMs. A re-run without `--airgapped` removes the proxy setting from the nodes again so that they install directly.

The jumpbox itself still needs internet access, and so does the machine you run OMS on. See [Dependencies and network requirements](#dependencies-and-network-requirements) for everything OMS downloads, and how to preinstall it.

#### Dependencies and network requirements

##### Jumpbox

The Codesphere installation commands (`oms install codesphere`, `oms install k0s`, `oms download package`, `oms copy package`) run on the jumpbox. `oms beta bootstrap-gcp` prepares it over SSH and does the following, each only when the item is missing:

| Item | How OMS provides it | Source | Preinstall to avoid the download |
|---|---|---|---|
| `oms` | Copied from the local machine when `--remote-oms-binary` is set; otherwise downloaded and moved to `/usr/local/bin/oms`. | `oms-portal.codesphere.com` | Install `oms` in the image, or pass `--remote-oms-binary`. |
| `sops` | Downloaded as v3.11.0 to `/usr/local/bin/sops`. | `github.com/getsops/sops/releases` | Install `sops` in the image so `sops` is in `PATH`. |
| `age`, `age-keygen` | Downloaded as the **latest** release and moved to `/usr/local/bin/`. The version is not pinned. | `dl.filippo.io/age` | Install `age` and `age-keygen` in the image so `age-keygen` is in `PATH`. |
| `curl`, `jq`, `tar`, `sudo` | Not installed. The downloads above need them, and the readiness checks for the registry and package proxy need `curl`. | OS package repositories | Install them in the image. |
| `podman` | Installed with the OS package manager when an air-gapped installation needs the local registry or the package proxy. | OS package repositories | Install `podman` in the image. |
| `apache2-utils` (Ubuntu) or `httpd-tools` (RHEL), `openssl` | `htpasswd` and `openssl` create the registry credentials and its self-signed certificate. `httpd-tools` or `apache2-utils` is installed together with `podman` for the registry; `openssl` is used but not installed. | OS package repositories | Install them in the image. |

The jumpbox also pulls container images with `podman` from `docker.io`: `docker.io/library/registry:3` for the local container registry (the default with `--airgapped`) and `docker.io/ubuntu/squid:6.6-24.04_beta` for the package proxy (`--airgapped` only). In an air-gapped environment, preload both images into the jumpbox's local container storage; OMS has no setting for a different source.

Commands that run on the jumpbox download further artifacts:

| Command | Downloads | Source |
|---|---|---|
| `oms download package` | The Codesphere installer package, with checksum verification. | The OMS portal (`OMS_PORTAL_API`, default `https://oms-portal.codesphere.com/api`), authenticated with `OMS_PORTAL_API_KEY`. |
| `oms copy package` | The container images and Helm charts that the package references, copied into your registry. | The upstream registry for Codesphere images, authenticated with registry credentials. No Docker daemon is needed. |
| `oms download k0s` | `k0s`, `k0sctl`, and with `--airgapped` the k0s airgap image bundle. When no k0s version is given, the current stable version is looked up first. | `github.com/k0sproject/k0s/releases`, `github.com/k0sproject/k0sctl/releases`, `api.github.com`, `docs.k0sproject.io/stable.txt` |
| `oms install k0s --single` | Nothing; it installs the `k0s` binary from the cache and uses `systemctl`. | None |
| `oms install codesphere` | Nothing from the internet apart from the registry pulls below. The installer Node.js runtime, `sops`, container runtime dependencies and the Argo CD chart come from the installer package. | The installer package |
| `oms update oms` | A new `oms` release. | `github.com` and `api.github.com` |

`oms install k0s` downloads `k0sctl` and, unless already cached, `k0s`. Downloads are cached in the user cache directory, `~/.cache/oms` on Linux, so running `oms download k0s --airgapped` first, or copying that directory to the jumpbox, avoids the GitHub downloads. The installer package and its extracted contents live in `OMS_WORKDIR` (default `./oms-workdir`).

##### Nodes

| Item | How OMS provides it | Source |
|---|---|---|
| Docker (RHEL only) | `oms beta bootstrap-gcp` installs `docker-ce` from Docker's RHEL 9 repository. On Ubuntu the Codesphere installer installs Docker itself. | `download.docker.com`, or the package proxy when air-gapped |
| `lvm2`, `container-selinux` (RHEL only) | Installed by `oms beta bootstrap-gcp`. | OS package repositories |
| k0s | `k0sctl` uploads the cached `k0s` binary to every node. With `--airgapped`, it also uploads the k0s airgap image bundle to the worker nodes, which provides the k0s system images. | Jumpbox cache |
| Codesphere images and charts | Pulled by the nodes from the configured registry. With `--airgapped` this is the registry on the jumpbox. | Configured registry |

The GCP-only k0s configuration script also applies the GCP cloud controller manager manifest from `raw.githubusercontent.com/kubernetes/cloud-provider-gcp`. In an air-gapped bootstrap the control plane node must therefore reach that host, or the script must be changed to apply a mirrored copy.

##### Network access summary

Hosts OMS contacts when it runs without `--airgapped`:

| Host | Used by | Contacted from |
|---|---|---|
| `oms-portal.codesphere.com` (or your `OMS_PORTAL_API`) | `download package`, `list packages`, API key commands | Jumpbox or local machine |
| `ghcr.io` | `copy package`, Codesphere image and chart pulls | Jumpbox, nodes |
| `api.github.com`, `github.com` | `oms` and `sops` downloads, k0s and k0sctl downloads, `update oms` | Jumpbox, local machine |
| `dl.filippo.io` | `age` download | Jumpbox |
| `docs.k0sproject.io` | k0s version lookup | Jumpbox, local machine |
| `raw.githubusercontent.com` | GCP cloud controller manager manifest | Control plane node |
| `docker.io` | Registry and Squid images for `--airgapped` | Jumpbox |
| `download.docker.com` | Docker on RHEL | Nodes (through the proxy when air-gapped) |
| OS package repositories | Package installation | All hosts (through the proxy when air-gapped) |
| `charts.rook.io`, `argoproj.github.io` | `bootstrap-local` Rook chart, and an explicit `--argo-version` override with `--argo-repo` | Machine that runs `bootstrap-local` |
| `*.googleapis.com`, `storage.googleapis.com` | `bootstrap-gcp`, GCS object storage | Local machine, Codesphere cluster |
| ACME server (`acme-v02.api.letsencrypt.org` by default) | Certificate issuance | Codesphere cluster |

An installation that avoids these downloads needs, at a minimum, `oms`, `sops` and `age-keygen` preinstalled on the jumpbox, the installer package and the k0s artifacts copied or cached ahead of time, the registry and Squid images preloaded into `podman`, and a registry that holds the mirrored Codesphere images. `oms install codesphere --package <archive>` installs from a local package.

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

## Select the Codesphere version through config.yaml

Set the package version alongside the existing operation skip steps:

```yaml
operations:
  codesphere:
    version: 1.111.0
    hash: abc123 # Optional: omit to select the latest build of this version.
  skip: [load-container-images]
```

```sh
oms install codesphere --config config.yaml --priv-key age-key.txt
```

The install command resolves the configured build through the package portal and
downloads its `installer-lite.tar.gz` archive into `OMS_WORKDIR` (default
`./oms-workdir`). Existing archives use the shared downloader's resume support,
and the checksum is verified after downloading.
Plain versions such as `1.95.0` resolve to portal tags such as `codesphere-v1.95.0`.
The install command also supports merged configuration files; the last configured
version wins. Existing skip-step behavior is unchanged.

An explicit `oms install codesphere --package <archive>` overrides the configured
version and supports offline installation. `oms download package <version>` and
`--version <version>` retain their existing behavior of downloading to the current
directory and continue requiring an explicit version.

The existing scalar form, `operations: {codesphere: 1.111.0}`, remains supported.
Config generation writes the explicit object form. Branch names such as `master`
are supported in either form. A supplied full Git hash can also match the abbreviated hash stored by the portal.
Quote numeric-looking versions such as `"1.10"` to preserve them as YAML strings.

```sh
oms init install-config --codesphere-version 1.111.0 --codesphere-hash abc123
```

`--codesphere-hash` is optional. In interactive mode these flags provide defaults for
the version and hash questions; entered answers take precedence. In non-interactive
mode they set the generated values directly. Bootstrap's `--install-version` and optional
`--install-hash` are also written to the generated config. GCP installations then
read that config instead of passing a package override; local package overrides
and the download required before registry mirroring remain supported.

When a bootstrap install config contains `operations.codesphere.version`, bootstrap
automatically installs that version using the optional configured hash. Explicit
`--install-version` or `--install-local` options override the config selection;
`--install-hash` overrides the configured hash.
