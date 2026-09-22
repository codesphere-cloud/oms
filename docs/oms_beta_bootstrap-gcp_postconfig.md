## oms beta bootstrap-gcp postconfig

Run post-configuration steps for GCP bootstrapping

### Synopsis

After bootstrapping GCP infrastructure, this command runs additional configuration steps
to finalize the setup for the Codesphere cluster on GCP:

* Install Google Cloud Controller Manager for ingress management.

```
oms beta bootstrap-gcp postconfig [flags]
```

### Options

```
      --age-key string               Path to the age private key (required for sops unless SOPS_AGE_KEY or SOPS_AGE_KEY_FILE is set)
  -h, --help                         help for postconfig
      --install-config-path string   Path to the installation configuration file (default "config.yaml")
      --private-key-path string      Path to the GCP service account private key file (optional)
      --secrets-file string          Path to the secrets (vault) file (default "prod.vault.yaml")
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms beta bootstrap-gcp](oms_beta_bootstrap-gcp.md)	 - Bootstrap GCP infrastructure for Codesphere

