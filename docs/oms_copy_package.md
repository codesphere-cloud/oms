## oms copy package

Copy all images and Helm charts from an installer package

### Synopsis

Read all container images and OCI Helm charts from an installer package BOM
and copy them to another registry.

Use --package for a local installer package or --version to download one
from the OMS portal. The source repository paths are preserved below --dest.

```
oms copy package [flags]
```

### Examples

```
# Copy artifacts from a local package
$ oms copy package --package codesphere-v1.70.0-installer-lite.tar.gz --dest registry.example.com/mirror

# Download an upstream package and copy without prompting
$ oms copy package --version codesphere-v1.70.0 --dest registry.example.com/mirror --yes

```

### Options

```
      --dest string      Destination registry or repository prefix
  -f, --file string      Installer artifact to download for an upstream package (default "installer-lite.tar.gz")
      --force            Re-extract the installer package
  -H, --hash string      Build hash used to disambiguate an upstream package version
  -h, --help             help for package
      --insecure         Allow image references to be fetched without TLS
  -p, --package string   Path to a local installer package
      --show-artifacts   Print the source and destination of every artifact to copy
  -v, --verbose          Enable debug logs
  -V, --version string   Codesphere package version to download from the OMS portal
  -y, --yes              Copy without prompting for confirmation
```

### SEE ALSO

* [oms copy](oms_copy.md)	 - Copy resources between locations

