# bootstrap-gcp

[`bootstrap-gcp.yml`](bootstrap-gcp.yml) is a reusable workflow that bootstraps Codesphere with
OMS on a fresh GCP project in the PC testing folder, waits until Codesphere answers, and
deletes the project again. OMS runs it before every release, and other repositories such as
`codesphere-monorepo` call it to test their builds, so a change to the bootstrap is made once
and every caller gets it with the next Renovate bump.

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

The caller job has to grant `id-token: write`, which the workflow uses to authenticate to
GCP through workload identity federation. A called workflow can't have more permissions
than its caller.

The workflow checks nothing out and uses no local actions, because both would resolve in the
caller's repository. An OMS build from source therefore comes from the caller as an artifact,
and notifications are up to the caller.

## Examples

A Codesphere build from a pull request, kept for QA when it carries a label, with a
self-hosted runner. Here a later job uses the URL:

```yaml
jobs:
  install:
    permissions:
      contents: read
      id-token: write
    uses: codesphere-cloud/oms/.github/workflows/bootstrap-gcp.yml@<commit sha> # v5.48.0
    secrets: inherit
    with:
      install-version: ${{ needs.package.outputs.release_version }}
      install-hash: ${{ needs.package.outputs.hash }}
      oms-version: v5.48.0
      prevent-destroy: ${{ contains(github.event.pull_request.labels.*.name, 'ci-pc-deploy-lite') }}
      github-team-org: codesphere-cloud
      github-team-slug: dev
      runs-on: '["self-hosted","fat","active"]'

  test:
    needs: install
    runs-on: ubuntu-latest
    steps:
      - run: echo "Testing ${{ needs.install.outputs.deployment-url }}"
```

An OMS build from source, against the latest Codesphere release, as `tag-release.yml` does.
The binary is passed as an artifact of the same run:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<commit sha> # v7
      - uses: actions/setup-go@<commit sha> # v7
        with:
          go-version-file: go.mod
      - run: make build-cli
      - uses: actions/upload-artifact@<commit sha> # v5
        with:
          name: oms
          path: oms

  bootstrap:
    needs: build
    permissions:
      contents: read
      id-token: write
    uses: ./.github/workflows/bootstrap-gcp.yml
    secrets: inherit
    with:
      oms-artifact: oms
      name: main
      datacenter-name: oms-main
```

An airgapped installation on RHEL 9, as `bootstrap-gcp-nightly.yml` does:

```yaml
    with:
      airgapped: true
      os: rhel-9
      root-disk-size: '200'
```

### Reporting a failure

The workflow posts nothing itself: where a failure goes is up to the caller. Its outputs
are set even when it fails. `failure-stage`, `failure-component` and `failure-cause` say
where and why the bootstrap failed, and the cause has every secret of the workflow
redacted, so it can go to Slack as it is:

```yaml
  notify:
    needs: install
    if: failure()
    runs-on: ubuntu-latest
    steps:
      - env:
          SLACK_WEBHOOK: ${{ secrets.SLACK_WEBHOOK }}
          STAGE: ${{ needs.install.outputs.failure-stage }}
          COMPONENT: ${{ needs.install.outputs.failure-component }}
          CAUSE: ${{ needs.install.outputs.failure-cause }}
          RUN_URL: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
        run: |
          text="Bootstrap failed at ${STAGE:-an unknown stage}${COMPONENT:+, component $COMPONENT}: $CAUSE"$'\n'"$RUN_URL"
          curl -fsS -H 'Content-type: application/json' \
            --data "$(jq -n --arg text "$text" '{text: $text}')" "$SLACK_WEBHOOK"
```

## Inputs

| input | default | |
| --- | --- | --- |
| `install-version` | empty | Codesphere version to install, e.g. `codesphere-v1.2.3` or a monorepo branch. Empty means the latest official `codesphere-vX.Y.Z` on the OMS portal. |
| `install-hash` | empty | package hash of `install-version` when it has several. Empty means the latest. |
| `oms-version` | empty | OMS release tag to install with. Empty means the latest release. |
| `oms-artifact` | empty | name of an artifact of this run with a linux/amd64 `oms` binary, used instead of a release. Can't be combined with `oms-version`. |
| `name` | `install-version` | name of the GCP project. Falls back to `<repository>-<run number>`. It is lowercased, everything but `a-z0-9` becomes `-`, and it is cut to 20 characters, which leaves room for the suffix the bootstrap appends. |
| `datacenter-id` | `42` | datacenter ID of the installation |
| `datacenter-name` | the branch | datacenter name to register, with `/` replaced by `-` |
| `ttl` | `2h` | TTL of the project, e.g. `30m` or `6h` |
| `prevent-destroy` | `false` | keep the project after the run, until its TTL expires |
| `require-existing-project` | `false` | fail if the project doesn't exist yet, instead of bootstrapping a fresh one |
| `codesphere-only` | `false` | re-install only Codesphere into an existing project, leaving k0s, Postgres, Ceph and the other dependencies alone |
| `airgapped` | `false` | bootstrap an airgapped installation (`--airgapped`). It uses the self-signed ingress CA instead of ACME, so the health check skips certificate verification. |
| `os` | `ubuntu` | operating system of all VMs (`--os`), e.g. `rhel-9` |
| `root-disk-size` | empty | root disk size of the VMs in GB. Empty keeps the OMS default. |
| `wait-for-healthy` | `true` | wait up to 15 minutes for `https://cs.<domain>/ide-service/healthz` to answer 200 |
| `github-team-org`, `github-team-slug` | empty | GitHub team that may log in through the GitHub app. Left out when empty. |
| `k0s-version` | `v1.36.6+k0s.0` | k0s version to install |
| `runs-on` | `"ubuntu-latest"` | the job's runner as JSON, e.g. `'["self-hosted","fat","active"]'` |

The test environment is fixed: projects live under `*.oms-testing.codesphere-demo.com` in the
PC testing folder and report to the dev OpenBao, OTel, Loki, Prometheus and identity
provider.

## Outputs

| output | |
| --- | --- |
| `deployment-url` | `https://cs.<project>.oms-testing.codesphere-demo.com` |
| `project-name` | project name after sanitizing |
| `base-domain` | base domain of the installation |
| `datacenter-name` | datacenter name it registered |
| `install-version` | Codesphere version installed, after resolving the latest |
| `oms-version` | OMS release tag used, or `artifact:<name>` |
| `failure-stage`, `failure-component`, `failure-cause` | where and why the bootstrap failed; empty when it didn't |

## Secrets

The workflow declares its secrets under the names the organization already uses, so a
repository that has them passes `secrets: inherit`. A repository that keeps them under
other names maps them one by one under `secrets:`. All of them are required:

- `OMS_INFRA_WIF_PROVIDER`, `OMS_INFRA_WIF_SA`: workload identity federation into GCP
- `PC_INSTALLER_TEST_GCP_FOLDER`, `PC_INSTALLER_TEST_GCP_BILLING_ACCOUNT`
- `PC_INSTALLER_TEST_GCP_SSH_KEY_PRIVATE`, `PC_INSTALLER_TEST_GCP_SSH_KEY_PUBLIC`: a fixed
  key pair, so a re-run can still reach the project
- `PC_INSTALLER_TEST_OMS_PORTAL_API_KEY`
- `PC_INSTALLER_TEST_GCP_OPENBAO_PASSWORD`
- `PC_INSTALLER_TEST_GCP_TEST_USER_PASSWORD`, `PC_INSTALLER_TEST_GCP_TEST_USER_PASSWORD_HASHED`
- `PC_INSTALLER_TEST_GCP_GITHUB_APP_NAME`, and `_CLIENT_ID` / `_CLIENT_SECRET` of
  `PC_INSTALLER_TEST_GCP_GITHUB_APP`, `_GITLAB_APP`, `_BITBUCKET_APP` and `_AZURE_DEVOPS_APP`
- `PC_INSTALLER_TEST_GCP_REGISTRY_USER`, `PC_INSTALLER_TEST_GCP_REGISTRY_PAT`
- `PC_INSTALLER_TEST_GCP_OIDC_CLIENT_ID`, `PC_INSTALLER_TEST_GCP_OIDC_CLIENT_SECRET`
- `PC_INSTALLER_TEST_GCP_EXTERNAL_LOKI_SECRET`
- `CENTRAL_OTEL_USERNAME`, `CENTRAL_OTEL_PASSWORD`
- `PROMETHEUS_REMOTE_WRITE_USERNAME`, `PROMETHEUS_REMOTE_WRITE_PASSWORD`

`secrets: inherit` passes only the secrets of the caller's repository and organization,
and only one level down.

## Versions and Renovate

This repository is public, so any workflow can call this one without access settings or a
PAT. Pin it to the commit SHA of an OMS release and put the tag in a comment, as with
actions. Renovate's `github-actions` manager then bumps it like any other `uses:`:

```yaml
uses: codesphere-cloud/oms/.github/workflows/bootstrap-gcp.yml@<commit sha> # v5.48.0
```

Don't call it at `@main`. A change here would then reach every caller untested, and
the run could not be repeated.

Pin `oms-version` too: an empty one takes whatever OMS was released last. Using the same
tag as the workflow keeps the workflow and the binary it was tested with together. Renovate
doesn't see an input by itself, so add this to the caller's `renovate.json`:

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

## Concurrency and cleanup

Runs for the same `name` (or `install-version`, if no name is given) wait for each other
under the concurrency group `pc-testing-gcp-<name>`, because they would share a project.
They are never cancelled: a cancelled run skips the cleanup and leaves the project behind.

If the project already exists, the workflow installs on top of it with
`--recover-config` and doesn't generate a new install config. A caller can install
one version and then update to another under the same `name`, with `prevent-destroy` on
the first run. In the follow-up run, `require-existing-project` makes sure a deleted
project fails the run instead of silently turning into a fresh install.

The project is deleted at the end unless `prevent-destroy` is set, also when the
bootstrap failed partway. A kept or leaked project is deleted when its TTL expires.

## Testing a change

A run needs the GCP secrets of the PC testing folder and takes up to three hours.
`bootstrap-gcp-testing.yml` runs it on pull requests here that change one of the
`bootstrap-gcp*.yml` workflows, and on demand with `gh workflow run bootstrap-gcp-testing.yml --ref <branch>`.
To test a change from another repository, point one of its jobs at your branch,
`codesphere-cloud/oms/.github/workflows/bootstrap-gcp.yml@<branch>`, and run it.

## Migrating from a copy

- OMS v5.38.2 or later is required. The workflow no longer passes `--vault-type=plain` to
  `oms init install-config`, the workaround that oms#835 made unnecessary.
- Input names use `-` instead of `_`: `install_version` becomes `install-version`, and so on.
- The job is named `Bootstrap Codesphere on GCP`. Required checks and scripts that look up
  the old job name, such as `Test PC Installer on GCP`, need to be updated.
- The monorepo reads the Prometheus credentials from 1Password. They need to be
  `PROMETHEUS_REMOTE_WRITE_USERNAME` and `PROMETHEUS_REMOTE_WRITE_PASSWORD` secrets
  instead, as in `oms`.
- The workflow posts no PR comment, Slack message or Loki links. The monorepo keeps these
  in a job after the call, built from the outputs.
