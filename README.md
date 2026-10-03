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

For an all-in-one GCP test installation, `oms beta bootstrap-gcp-single-vm` creates one Spot VM with an extra Ceph disk, runs `oms install k0s --single`, and runs `bootstrap-local --expose-shared` on the VM using the installer archive. Repeat `--config` to merge local config.yaml files in order; use `--vault` and `--priv-key` when they reference SOPS vault secrets. The command uploads the config files and applies VM-specific settings to the result. `--install-config-template` remains available for one initial base file. The local bootstrap runs `oms install codesphere dependencies` and then `oms install codesphere platform`; the dependencies phase installs Argo CD. It writes the effective config to `single-vm-config.yaml` in the OMS workdir, creates Google Public CA EAB credentials in `single-vm-vault.yaml`, and provides both files to the VM. The VM's one external IP serves HTTPS on port 443, HTTP on port 80, and the SSH workspace proxy on port 2222; VM administration stays on port 22. Set `OMS_REGISTRY_PASSWORD` and provide `--project-name`, `--billing-account`, `--base-domain`, `--remote-oms-binary` (a Linux amd64 build of this OMS version), `--registry-user`, and either `--install-local` for an archive on your machine or `--install-version` to download one through the OMS portal. The extra disk defaults to the Rook device filter `^sdb$`; use `--ceph-device-filter` if it has another name. The GCP account must be allowed to create projects, VMs, service accounts, DNS records, and Public CA external account keys. The command writes the usual `gcp-infra.json`, and `oms beta bootstrap-gcp-single-vm cleanup` removes the project and DNS records. This workflow is for testing and the Spot VM can be interrupted.

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
