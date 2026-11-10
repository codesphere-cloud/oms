## oms build images

Build and push container images

### Synopsis

Build and push container images based on the configuration file.
Extracts necessary image configurations from the provided install config and the downloaded package.

```
oms build images [flags]
```

### Options

```
  -c, --config string        Path to the configuration YAML file
  -f, --force[=true|false]   Force new unpacking of the package even if already extracted
  -h, --help[=true|false]    help for images
```

### Options inherited from parent commands

```
      --verbose[=true|false]   Enable verbose output
```

### SEE ALSO

* [oms build](oms_build.md)	 - Build and push images to a registry

