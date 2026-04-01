## oms beta bootstrap-gcp restart-vms

Restart stopped or terminated GCP VMs

### Synopsis

Restarts GCP compute instances that were stopped or terminated,
for example after spot VM preemption.
By default, restarts all VMs defined in the infrastructure.
Use --name to restart a single VM.
Project ID and zone are read from the local infra file if available,
or can be specified via flags.

```
oms beta bootstrap-gcp restart-vms [flags]
```

### Examples

```
# Restart all VMs using project info from the local infra file
$ oms beta bootstrap-gcp restart-vms

# Restart only the jumpbox VM
$ oms beta bootstrap-gcp restart-vms --name jumpbox

# Restart a specific k0s node
$ oms beta bootstrap-gcp restart-vms --name k0s-1

# Restart all VMs with explicit project and zone
$ oms beta bootstrap-gcp restart-vms --project-id my-project --zone us-central1-a

# Restart a specific VM with explicit project and zone
$ oms beta bootstrap-gcp restart-vms --project-id my-project --zone us-central1-a --name ceph-1

```

### Options

```
  -h, --help                help for restart-vms
      --name string         Name of a specific VM to restart (e.g. jumpbox, postgres, ceph-1, k0s-1). Restarts all VMs if not specified.
      --project-id string   GCP Project ID (optional, will use infra file if not provided)
      --zone string         GCP Zone (optional, will use infra file if not provided)
```

### SEE ALSO

* [oms beta bootstrap-gcp](oms_beta_bootstrap-gcp.md)	 - Bootstrap GCP infrastructure for Codesphere

