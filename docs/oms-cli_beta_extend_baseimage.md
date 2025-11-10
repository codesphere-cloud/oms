## oms-cli beta extend baseimage

Extend Codesphere's workspace base image for customization

### Synopsis

Loads the baseimage from Codesphere package and generates a Dockerfile based on it.
This enables you to extend Codesphere's base image with specific dependencies.

To use the custom base image, you need to push the resulting image to your container registry and
reference it in your install-config for the Codesphere installation process to pick it up and include it in Codesphere

```
oms-cli beta extend baseimage [flags]
```

### Options

```
  -b, --baseimage string    Base image file name inside the package to extend (default: 'workspace-agent-24.04') (default "workspace-agent-24.04")
  -d, --dockerfile string   Output Dockerfile to generate for extending the base image (default "Dockerfile")
  -f, --force               Enforce package extraction
  -h, --help                help for baseimage
  -p, --package string      Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load base image from
```

### SEE ALSO

* [oms-cli beta extend](oms-cli_beta_extend.md)	 - Extend Codesphere ressources such as base images.

