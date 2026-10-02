## oms test codesphere

Run a playlist of tests against a Codesphere installation

### Synopsis

Run a playlist of tests against a Codesphere installation.

A playlist is an ordered selection of test steps, for example a status report
followed by a smoke test. Every step is run even if an earlier one failed,
unless --fail-fast is set, and the results are summarized at the end.

Run 'oms test list' to see the available steps and playlists.

This replaces 'oms smoketest codesphere', which is deprecated. The smoke test is
the 'smoketest' step here, so 'oms test codesphere --tests smoketest' runs exactly
what that command did.

```
oms test codesphere [flags]
```

### Examples

```
# Run the "default" playlist against a Codesphere installation
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN

# Run a specific playlist
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --playlist readiness

# Run a specific list of steps, in the given order
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --tests status,smoketest

# Wait for the installation to become ready before running the remaining steps
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --wait

# Stop at the first failing step instead of running the whole playlist
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --fail-fast

```

### Options

```
      --baseurl string          Base URL of the Codesphere API
      --fail-fast               Skip the remaining steps after the first failure
  -h, --help                    help for codesphere
      --plan-id string          Plan ID to use for workspaces created by tests
      --playlist string         Playlist of steps to run (default,readiness) (default "default")
      --profile string          CI profile to use for landscape and pipeline (default "ci.yml")
  -q, --quiet                   Suppress progress logging
      --team-id string          Team ID to run tests in
      --tests strings           Comma-separated list of steps to run, in the given order (smoketest,status). Takes precedence over --playlist.
      --timeout duration        Timeout for the entire test run (default 20m0s)
      --token string            API token for authentication
      --wait                    Wait for the installation to become ready during the status step
      --wait-timeout duration   Timeout when waiting for the installation to become ready (default 5m0s)
```

### Options inherited from parent commands

```
      --verbose   Enable verbose output
```

### SEE ALSO

* [oms test](oms_test.md)	 - Run playlists of tests against Codesphere components

