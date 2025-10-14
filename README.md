[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![CLI GitHub Workflow Status](https://github.com/codesphere-cloud/oms/actions/workflows/service-build_test.yml/badge.svg)
![Service GitHub Workflow Status](https://github.com/codesphere-cloud/oms/actions/workflows/cli-build_test.yml/badge.svg)

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
gh release download -R codesphere-cloud/oms -O /usr/local/bin/oms-cli -p "oms-cli*darwin_arm64"
chmod +x /usr/local/bin/oms-cli
```

##### Linux Amd64

```
gh release download -R codesphere-cloud/oms -O /usr/local/bin/oms-cli -p "oms-cli*linux_amd64"
chmod +x /usr/local/bin/oms-cli
```

#### Using `wget`

This option requires to have the `wget` and `jq` utils installed. Download the OMS CLI and add permissions to run it with the following commands:
Note that some commands may require you to elevate to the root user with `sudo`.

##### ARM Mac

```
wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match("oms-cli.*darwin_arm64")) | .browser_download_url' | xargs wget -O oms-cli
mv oms-cli /usr/local/bin/oms-cli
chmod +x /usr/local/bin/oms-cli
```

##### Linux Amd64

```
wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match("oms-cli.*linux_amd64")) | .browser_download_url' | xargs wget -O oms-cli
mv oms-cli /usr/local/bin/oms-cli
chmod +x /usr/local/bin/oms-cli
```

#### Manual Download

You can also download the pre-compiled binaries from the [OMS Releases page](https://github.com/codesphere-cloud/oms/releases).
Note that some commands may require you to elevate to the root user with `sudo`.

1. Go to the [latest release](https://github.com/codesphere-cloud/oms-cli/releases/latest).

2. Download the appropriate release for your operating system and architecture (e.g., `oms-cli_darwin_amd64` for macOS, `oms-cli_linux_amd64` for Linux, or `oms-cli_windows_amd64` for Windows).

3. Move the `oms-cli` binary to a directory in your system's `PATH` (e.g., `/usr/local/bin` on Linux/Mac, or a directory added to `Path` environment variable on Windows).

4. Make the binary executable (e.g. by running `chmod +x /usr/local/bin/oms-cli` on Mac or Linux)

#### Available Commands

The OMS CLI organizes its functionality into several top-level commands, each with specific subcommands and flags.

See our [Usage Documentation](docs) for usage information about the specific subcommands.

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
