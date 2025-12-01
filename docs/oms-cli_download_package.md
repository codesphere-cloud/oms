## oms-cli download package

Download a codesphere package

### Synopsis

Download a specific version of a Codesphere package
To list available packages, run oms list packages.

```
oms-cli download package [VERSION] [flags]
```

### Examples

```
# Download Codesphere version 1.55.0
$ oms-cli download package codesphere-v1.55.0

# Download Codesphere version 1.55.0
$ oms-cli download package --version codesphere-v1.55.0

# Download lite package of Codesphere version 1.55.0
$ oms-cli download package --version codesphere-v1.55.0 --file installer-lite.tar.gz

```

### Options

```
  -f, --file string      Specify artifact to download (default "installer.tar.gz")
  -H, --hash string      Hash of the version to download if multiple builds exist for the same version
  -h, --help             help for package
  -q, --quiet            Suppress progress output during download
  -V, --version string   Codesphere version to download
```

### SEE ALSO

* [oms-cli download](oms-cli_download.md)	 - Download resources available through OMS

