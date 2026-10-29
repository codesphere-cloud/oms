## oms download k0s

Download k0s Kubernetes distribution

### Synopsis

Download the k0s and k0sctl binaries to the OMS cache.
Defaults to the versions 'oms install k0s' expects, so a later installation finds
them in the cache and does not need internet access.

```
oms download k0s [flags]
```

### Examples

```
# Download k0s using the Go-native implementation
$ oms download k0s

# Download a specific version of k0s
$ oms download k0s --version 1.22.0

# Download a specific version of k0sctl
$ oms download k0s --k0sctl-version v0.33.1

# Force download even if k0s binary exists
$ oms download k0s --force

# Also download the airgap image bundle for that version into the OMS cache
$ oms download k0s --airgapped

# Also download the airgap image bundle for that version to the given file
$ oms download k0s --airgapped --path <file>

```

### Options

```
      --airgapped               Also download the airgap image bundle for that version
  -f, --force                   Force download even if k0s binary exists
  -h, --help                    help for k0s
      --k0sctl-version string   Version of k0sctl to download (default "v0.33.1")
      --path string             File to save the airgap image bundle to (requires --airgapped; default: the OMS cache)
  -v, --version string          Version of k0s to download (default "v1.31.14+k0s.0")
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms download](oms_download.md)	 - Download resources available through OMS

