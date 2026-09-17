## oms status codesphere

Check the status of a Codesphere installation

### Synopsis

Check whether a Codesphere installation is reachable and ready to use,
by querying the Codesphere API.

```
oms status codesphere [flags]
```

### Examples

```
# Check the status of a Codesphere installation
$ oms status codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN

# Block and retry until the Codesphere installation is ready
$ oms status codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --wait

```

### Options

```
      --baseurl string     Base URL of the Codesphere API
  -h, --help               help for codesphere
      --timeout duration   Timeout when waiting for the installation to become ready (default 5m0s)
      --token string       API token for authentication
      --wait               Block and retry until the installation is ready
```

### SEE ALSO

* [oms status](oms_status.md)	 - Check the status of Codesphere components

