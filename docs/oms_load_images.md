## oms load images

Mirror all Codesphere OCI images required to install package from Codesphere's registry

### Synopsis

Mirror all Codesphere OCI images required to install package from Codesphere's registry into a target registry.
This is required for installations that require a custom registry, such as air-gapped environments.

Ensure that the target registry is reachable and that you have permission to push images to it.
Registry authentication is read from local container registry credentials, such as Docker config,
Docker credential helpers, or Podman auth files.

Logging in to the source and target registry before running this command is required.

To use the custom registry, it must be configured using Codesphere's configuration file before installing Codesphere.

```
oms load images <package> <target-registry> [flags]
```

### Examples

```
# Mirror every Codesphere OCI image required for Codesphere 1.68.0 into the target registry
$ oms load images codesphere-v1.68.0.tar.gz registry.internal.example.com

```

### Options

```
      --dry-run   Print planned copy operations without copying images
  -f, --force     Force new package extraction even if already extracted
  -h, --help      help for images
```

### SEE ALSO

* [oms load](oms_load.md)	 - Load resources into a local or custom registry

