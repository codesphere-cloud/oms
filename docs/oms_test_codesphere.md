## oms test codesphere

Run a playlist of tests against a Codesphere installation

### Synopsis

Run a playlist of tests against a Codesphere installation.

A playlist is an ordered selection of tests, for example a status report
followed by a smoke test. Every test is run even if an earlier one failed,
unless --fail-fast is set, and the results are summarized at the end.

Run 'oms test list' to see the available tests and playlists.

```
oms test codesphere [flags]
```

### Examples

```
# Run the "default" playlist against a Codesphere installation
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN

# Run a specific playlist
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --playlist readiness

# Run a specific list of tests, in the given order
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --tests status,smoketest

# Wait for the installation to become ready before running the remaining tests
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --wait

# Stop at the first failing test instead of running the whole playlist
$ oms test codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --fail-fast

```

### Options

```
      --baseurl string          Base URL of the Codesphere API
      --fail-fast               Skip the remaining tests after the first failure
  -h, --help                    help for codesphere
      --plan-id string          Plan ID to use for workspaces created by tests
      --playlist string         Playlist of tests to run (default,readiness) (default "default")
      --profile string          CI profile to use for landscape and pipeline (default "ci.yml")
  -q, --quiet                   Suppress progress logging
      --team-id string          Team ID to run tests in
      --tests strings           Comma-separated list of tests to run, in the given order (status,smoketest). Takes precedence over --playlist.
      --timeout duration        Timeout for the entire test run (default 20m0s)
      --token string            API token for authentication
      --wait                    Wait for the installation to become ready during the status test
      --wait-timeout duration   Timeout when waiting for the installation to become ready (default 5m0s)
```

### SEE ALSO

* [oms test](oms_test.md)	 - Run playlists of tests against Codesphere components

