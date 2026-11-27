# bootstrap-gcp

[`bootstrap-gcp.yml`](bootstrap-gcp.yml) is a reusable workflow that bootstraps Codesphere with
OMS on a fresh GCP project in the PC testing folder, waits until Codesphere answers, and
deletes the project again. OMS runs it before every release, and other repositories such as
`codesphere-monorepo` call it to test their builds. The inputs, outputs and secrets are
described in the workflow itself.

```yaml
jobs:
  bootstrap:
    permissions:
      contents: read
      id-token: write
    uses: codesphere-cloud/oms/.github/workflows/bootstrap-gcp.yml@<commit sha> # v5.48.0
    secrets: inherit
    with:
      install-version: codesphere-v1.2.3
      oms-version: v5.48.0
```

The caller job has to grant `id-token: write` for the workload identity federation into GCP.
The workflow checks nothing out and uses no local actions, because both would resolve in the
caller's repository. An OMS build from source therefore comes from the caller as an artifact
(`oms-artifact`), and notifications are up to the caller. In this repository,
[`bootstrap-gcp-oms.yml`](bootstrap-gcp-oms.yml) does both.

## Versions and Renovate

Pin the workflow to the commit SHA of an OMS release with the tag in a comment, as with
actions, so Renovate's `github-actions` manager bumps it. Don't call it at `@main`: a change
would reach every caller untested. Pin `oms-version` to the same tag, and let Renovate bump
it with a custom manager in the caller's `renovate.json`:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "managerFilePatterns": ["/^\\.github/workflows/.+\\.ya?ml$/"],
      "matchStrings": ["oms-version:\\s*[\"']?(?<currentValue>v\\d+\\.\\d+\\.\\d+)[\"']?"],
      "depNameTemplate": "codesphere-cloud/oms",
      "datasourceTemplate": "github-releases"
    }
  ]
}
```

## Reporting a failure

The outputs are set even when the bootstrap fails. `failure-stage`, `failure-component` and
`failure-cause` say where and why, and the cause has every secret redacted, so it can be
posted as it is:

```yaml
  notify:
    needs: bootstrap
    if: failure()
    runs-on: ubuntu-latest
    steps:
      - env:
          SLACK_WEBHOOK: ${{ secrets.SLACK_WEBHOOK }}
          STAGE: ${{ needs.bootstrap.outputs.failure-stage }}
          CAUSE: ${{ needs.bootstrap.outputs.failure-cause }}
        run: |
          text="Bootstrap failed at ${STAGE:-an unknown stage}: $CAUSE"
          curl -fsS -H 'Content-type: application/json' \
            --data "$(jq -n --arg text "$text" '{text: $text}')" "$SLACK_WEBHOOK"
```

## Concurrency and existing projects

Runs for the same `name` (or `install-version`) share a project, so they queue in the
concurrency group `pc-testing-gcp-<name>` and are never cancelled, since a cancelled run
would skip the cleanup. If the project already exists, the workflow installs on top of it
with `--recover-config`. To test an update, install the first version with `prevent-destroy`
and the second under the same `name` with `require-existing-project`, so a deleted project
fails the run instead of silently becoming a fresh install. Kept or leaked projects are
deleted when their TTL expires.

To test a change to the workflow, open a pull request here (`bootstrap-gcp-testing.yml` runs
it) or point a job in another repository at `bootstrap-gcp.yml@<branch>`.
