## oms download k0s

Download k0s Kubernetes distribution

### Synopsis

Download a k0s binary directly to the OMS workdir.
Will download the latest version if no version is specified.

```
oms download k0s [flags]
```

### Examples

```
# Download k0s using the Go-native implementation
$ oms download k0s

# Download a specific version of k0s
$ oms download k0s --version 1.22.0

# Force download even if k0s binary exists
$ oms download k0s --force

```

### Options

```
  -f, --force            Force download even if k0s binary exists
  -h, --help             help for k0s
  -v, --version string   Version of k0s to download
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms download](oms_download.md)	 - Download resources available through OMS

