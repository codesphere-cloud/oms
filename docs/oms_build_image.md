## oms build image

Build and push Docker image using Dockerfile and Codesphere package version

### Synopsis

Build a Docker image from a Dockerfile and push it to a registry, tagged with the Codesphere version from the package.

```
oms build image [flags]
```

### Examples

```
# Build image for Codesphere version 1.68.0 and push to specified registry
$ oms build image --dockerfile baseimage/Dockerfile --package codesphere-v1.68.0.tar.gz --registry my-registry.com/my-image

```

### Options

```
  -d, --dockerfile string    Path to the Dockerfile to build (required)
  -f, --force[=true|false]   Force new unpacking of the package even if already extracted
  -h, --help[=true|false]    help for image
  -p, --package string       Path to the Codesphere package (required)
  -r, --registry string      Registry URL to push to (e.g., my-registry.com/my-image) (required)
```

### Options inherited from parent commands

```
      --verbose[=true|false]   Enable verbose output
```

### SEE ALSO

* [oms build](oms_build.md)	 - Build and push images to a registry

