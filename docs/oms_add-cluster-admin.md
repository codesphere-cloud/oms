## oms add-cluster-admin

Set the cluster admin email in a Kubernetes secret

### Synopsis

Sets the cluster admin email in the target Kubernetes cluster by writing it to a
Kubernetes secret. The email is stored under the 'email' key of the secret, which the platform
deployment consumes via a secretKeyRef. The secret is created if it does not exist yet and
updated otherwise, so running the command again overwrites the previous email.

The target cluster is determined by the current kubeconfig context.

```
oms add-cluster-admin [flags]
```

### Examples

```
# Set the cluster admin email using the default secret and namespace
$ oms add-cluster-admin --email niklas@codesphere.com

# Set the cluster admin email in a custom namespace
$ oms add-cluster-admin --email admin@codesphere.com --namespace kube-system --secret-name cluster-admin-email

```

### Options

```
      --email string         Email address of the cluster admin (required)
  -h, --help                 help for add-cluster-admin
      --namespace string     Kubernetes namespace where the secret is stored (default "codesphere")
      --secret-name string   Name of the Kubernetes secret holding the cluster admin email (default "cluster-admin-email")
```

### SEE ALSO

* [oms](oms.md)	 - Codesphere Operations Management System (OMS)

