## oms-cli update install-config

Update an existing Codesphere installer configuration

### Synopsis

Update fields in an existing install-config after generating one initially.

This command allows you to modify specific configuration fields in an existing
config.yaml and prod.vault.yaml without regenerating everything. OMS will
automatically detect which dependent secrets and certificates need to be
regenerated based on the changes made.

For example, updating the PostgreSQL primary IP will trigger regeneration
of the PostgreSQL server certificates that include that IP address.

```
oms-cli update install-config [flags]
```

### Examples

```
# Update PostgreSQL primary IP and regenerate certificates
$ oms-cli update install-config --postgres-primary-ip 10.10.0.4 --config config.yaml --vault prod.vault.yaml

# Update Codesphere domain
$ oms-cli update install-config --domain new.example.com --config config.yaml --vault prod.vault.yaml

# Update Kubernetes API server host
$ oms-cli update install-config --k8s-api-server 10.0.0.10 --config config.yaml --vault prod.vault.yaml

```

### Options

```
      --ceph-nodes-subnet string                     Ceph nodes subnet
      --cluster-gateway-ips strings                  Cluster gateway IP addresses (comma-separated)
      --cluster-gateway-service-type string          Cluster gateway service type
      --cluster-public-gateway-ips strings           Cluster public gateway IP addresses (comma-separated)
      --cluster-public-gateway-service-type string   Cluster public gateway service type
  -c, --config string                                Path to existing config.yaml file (default "config.yaml")
      --custom-domains-cname-base-domain string      Custom domains CNAME base domain
      --dns-servers strings                          DNS servers (comma-separated)
      --domain string                                Main Codesphere domain
  -h, --help                                         help for install-config
      --k8s-api-server string                        Kubernetes API server host
      --k8s-pod-cidr string                          Kubernetes Pod CIDR
      --k8s-service-cidr string                      Kubernetes Service CIDR
      --postgres-primary-hostname string             Primary PostgreSQL server hostname
      --postgres-primary-ip string                   Primary PostgreSQL server IP
      --postgres-replica-ip string                   Replica PostgreSQL server IP
      --postgres-replica-name string                 Replica PostgreSQL server name
      --postgres-server-address string               PostgreSQL server address (for external mode)
      --public-ip string                             Codesphere public IP address
      --vault string                                 Path to existing prod.vault.yaml file (default "prod.vault.yaml")
      --with-comments                                Add helpful comments to the generated YAML files
      --workspace-hosting-base-domain string         Workspace hosting base domain
```

### SEE ALSO

* [oms-cli update](oms-cli_update.md)	 - Update OMS related resources

