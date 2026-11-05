## oms install bootstrap-tls

Issue bootstrap certificates on a connected GCP jumpbox

```
oms install bootstrap-tls [flags]
```

### Options

```
      --cache-dir string        Persistent ACME account and certificate directory (default "/root/oms-acme")
      --config string           Data center install config
      --dns-project string      Project hosting the public Cloud DNS zone
      --domain strings          Additional certificate DNS names
  -h, --help                    help for bootstrap-tls
      --vault string            Encrypted data center vault
      --vault-priv-key string   Age key file for the data center vault
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms install](oms_install.md)	 - Install Codesphere and other components

