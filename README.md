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

For an all-in-one GCP test installation, `oms beta bootstrap-gcp --single-vm` creates one VM with an extra Ceph disk, runs `oms install k0s --single`, and runs `bootstrap-local --expose-shared` on the VM using the installer archive. Both bootstrap modes accept `--install-config-template` as an initial overlay and repeated `--config` files merged in order before infrastructure settings; use `--vault` and `--priv-key` when they reference SOPS vault secrets. The command uploads the config files and applies VM-specific settings to the result. `--install-config-template` remains available for one initial base file. The local bootstrap runs `oms install codesphere dependencies` and then `oms install codesphere platform`; the dependencies phase installs Argo CD. It writes the effective config to `single-vm-config.yaml` in the OMS workdir, creates Google Public CA EAB credentials in `single-vm-vault.yaml`, and provides both files to the VM. The VM's one external IP serves HTTPS on port 443, HTTP on port 80, and the SSH workspace proxy on port 2222; VM administration stays on port 22. Set `OMS_REGISTRY_PASSWORD` and provide `--project-name`, `--billing-account`, `--base-domain`, `--registry-user`, and either `--install-local` for an archive on your machine or `--install-version` to download one through the OMS portal. The boot disk defaults to 50 GB in both modes; use `--root-disk-size` to override it. The extra 100 GB Ceph disk uses the fixed Rook device filter `^sdb$`. Both modes reuse OMS on the remote host or download the latest release when absent; optionally supply `--remote-oms-binary` to upload a local Linux amd64 build. Use `--machine-type` to override the machine type for every VM in either mode; omitting it preserves the existing sizes for each role. The GCP account must be allowed to create projects, VMs, service accounts, DNS records, and Public CA external account keys. The command writes the usual `gcp-infra.json`, and `oms beta bootstrap-gcp cleanup` removes the project and DNS records. This workflow is for testing. VMs use standard provisioning by default; pass `--spot-vms` to use a Spot VM, which can be interrupted and falls back to a standard VM if Spot capacity is unavailable.

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
