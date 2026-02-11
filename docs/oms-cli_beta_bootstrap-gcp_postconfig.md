## oms-cli beta bootstrap-gcp postconfig

Run post-configuration steps for GCP bootstrapping

### Synopsis

After bootstrapping GCP infrastructure, this command runs additional configuration steps
to finalize the setup for the Codesphere cluster on GCP:

* Install Google Cloud Controller Manager for ingress management.

```
oms-cli beta bootstrap-gcp postconfig [flags]
```

### Options

```
  -h, --help                         help for postconfig
      --install-config-path string   Path to the installation configuration file (default "config.yaml")
      --private-key-path string      Path to the GCP service account private key file (optional)
```

### SEE ALSO

* [oms-cli beta bootstrap-gcp](oms-cli_beta_bootstrap-gcp.md)	 - Bootstrap GCP infrastructure for Codesphere

