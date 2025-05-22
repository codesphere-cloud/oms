# Operations Management System - OMS

This repository contains the source for the operations management system. It
contains the sources for both the CLI and the Service. 

## CLI
The CLI tool is used to bootstrap Codesphere cluster on customer sites and
replaces the formerly used private cloud installer.

### How to Build?

```shell
make build-cli
```

### How to Test?


### How to Use?


## Service

### How to Build?

```shell
make build-service
```

### How to Test?


### How to Use?


## How to add a command to one of the binaries?

This project currently uses a fork of cobra-cli with locally-scoped variables: https://github.com/NautiluX/cobra-cli-local

```shell
cobra-cli add -L -d cli -p install postgres
```

This command will add the following command to the CLI:

```shell
oms-cli install postgres
```
