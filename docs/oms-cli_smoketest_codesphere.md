## oms-cli smoketest codesphere

Run smoke tests for a Codesphere installation

### Synopsis

Run automated smoke tests for a Codesphere installation by creating a workspace,
setting environment variables, executing commands, syncing landscape, and running a pipeline stage.
The workspace is automatically deleted after the test completes.

```
oms-cli smoketest codesphere [flags]
```

### Examples

```
# Run smoke tests against a Codesphere installation
$ oms-cli smoketest codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID

# Run smoke tests in quiet mode (no progress logging)
$ oms-cli smoketest codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --quiet

# Run smoke tests with custom timeout
$ oms-cli smoketest codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --timeout 15m

# Run only specific steps of the smoke test (workspace won't be deleted)
$ oms-cli smoketest codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --steps createWorkspace,syncLandscape

# Run specific steps and delete the workspace afterwards
$ oms-cli smoketest codesphere --baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --steps createWorkspace,syncLandscape,deleteWorkspace

```

### Options

```
      --baseurl string     Base URL of the Codesphere API
  -h, --help               help for codesphere
      --plan-id string     Plan ID for workspace creation
      --profile string     CI profile to use for landscape and pipeline (default "ci.yml")
  -q, --quiet              Suppress progress logging
      --steps strings      Comma-separated list of steps to run (createWorkspace,setEnvVar,createFiles,syncLandscape,startPipeline,deleteWorkspace). If empty, all steps including deleteWorkspace are run. If specified without deleteWorkspace, the workspace will be kept for manual inspection.
      --team-id string     Team ID for workspace creation
      --timeout duration   Timeout for the entire smoke test (default 10m0s)
      --token string       API token for authentication
```

### SEE ALSO

* [oms-cli smoketest](oms-cli_smoketest.md)	 - Run smoke tests for Codesphere components

