## oms load images

Copy all GHCR image and OCI chart references from a BOM into a target registry

### Synopsis

Extract the BOM from a Codesphere package, find all image and OCI chart
references that point to ghcr.io, and mirror them into a target registry.

```
oms load images <package> <target-registry> [flags]
```

### Examples

```
# Mirror every ghcr.io image and OCI chart reference from the package BOM into the target registry
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
