## oms-cli beta bootstrap-gcp cleanup

Clean up GCP infrastructure created by bootstrap-gcp

### Synopsis

Deletes a GCP project that was previously created using the bootstrap-gcp command.

```
oms-cli beta bootstrap-gcp cleanup [flags]
```

### Examples

```
  # Clean up using project ID from the local infra file
  oms beta bootstrap-gcp cleanup

  # Clean up a specific project
  oms beta bootstrap-gcp cleanup --project-id my-project-abc123

  # Force cleanup without confirmation (skips OMS-managed check)
  oms beta bootstrap-gcp cleanup --project-id my-project-abc123 --force

  # Skip DNS record cleanup
  oms beta bootstrap-gcp cleanup --skip-dns-cleanup
```

### Options

```
      --force               Skip confirmation prompt and OMS-managed check
  -h, --help                help for cleanup
      --project-id string   GCP Project ID to delete (optional, will use infra file if not provided)
      --skip-dns-cleanup    Skip cleaning up DNS records
```

### SEE ALSO

* [oms-cli beta bootstrap-gcp](oms-cli_beta_bootstrap-gcp.md)	 - Bootstrap GCP infrastructure for Codesphere

