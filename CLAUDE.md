# OMS Agent Notes

Generic agent behavior lives in `/Users/alessandroaffinito/.agents`. Do not duplicate those rules here. In particular, rely on:

- `/Users/alessandroaffinito/.agents/rules/worktree-isolation.mdc`
- `/Users/alessandroaffinito/.agents/rules/infra-secrets-hygiene.mdc`
- `/Users/alessandroaffinito/.agents/rules/infra-manual-approval-gates.mdc`
- `/Users/alessandroaffinito/.agents/rules/commit-signoff.mdc`
- `/Users/alessandroaffinito/.agents/rules/rigor-and-tone.mdc`
- `/Users/alessandroaffinito/.agents/rules/clean-history.mdc`
- `/Users/alessandroaffinito/.agents/rules/pre-push-gates.mdc`
- `/Users/alessandroaffinito/.agents/rules/pr-lifecycle.mdc`

Keep this file OMS-specific.

## Product Context

OMS is the operations management toolchain for bootstrapping, installing, updating, and packaging Codesphere on customer-managed infrastructure.

Codesphere's broader platform goal is infrastructure abstraction and portability across cloud and on-prem environments. In this repo, prefer reproducible operator workflows, keep provider-specific logic isolated, and avoid changes that increase Kubernetes or infrastructure complexity for users.

## Commands

- `make build-cli` - build the CLI from `cli/` and move the binary to repo root as `./oms`
- `make test` - run the repo-wide Go test sweep
- `make test-integration` - run integration-tagged CLI tests
- `make format` - run `go fmt ./...`
- `make lint` - run `golangci-lint`
- `make generate` - refresh mocks and generated artifacts
- `make docs` - regenerate Cobra markdown docs in `docs/`
- `make generate-license` - refresh `NOTICE` and license headers
- `make release-local VERSION=x.y.z` - local GoReleaser snapshot build

## Architecture

- `cli/main.go` - CLI entrypoint
- `cli/cmd/` - Cobra command tree and flag wiring
- `internal/bootstrap/` - bootstrap flows for local and provider-specific environments
- `internal/installer/` - install config, vault/secrets, Helm, and package installation helpers
- `internal/portal/` - OMS portal API client for builds and API key management
- `internal/env/` - environment variable defaults such as `OMS_PORTAL_API` and `OMS_WORKDIR`
- `docs/` - generated CLI docs
- `hack/` - docs, license, and release helper scripts

The current codebase center of gravity is the CLI plus shared installer/bootstrap libraries.

## Workflow

- Keep Cobra command parsing and UX wiring in `cli/cmd/*`; move non-trivial logic into `internal/*`.
- When command names, flags, or help text change, run `make docs`.
- When interfaces, mocks, or `go generate`-managed files change, run `make generate`.

## Environment And Secrets

- `OMS_PORTAL_API_KEY` is required for portal-backed operations.
- `OMS_PORTAL_API` defaults to `https://oms-portal.codesphere.com/api`; avoid accidentally hitting production during local testing.
- `OMS_WORKDIR` defaults to `./oms-workdir`.
- Local bootstrap may prompt for `OMS_REGISTRY_PASSWORD` if it is not already set.
- Treat `.installer/`, generated vault files, and kubeconfig-bearing secrets as sensitive material.

## Gotchas

- `README.md` and `CONTRIBUTING.md` contain some stale guidance; prefer the `Makefile` and GitHub Actions workflows as the source of truth.
- `make build-cli` writes the final binary to repo root as `./oms`.
- `cli/cmd/root.go` upgrades legacy 25-character API keys by calling the portal and printing a replacement export line.
- `make docs` generates the full Cobra command tree and copies `docs/oms.md` to `docs/README.md`.
- `make generate` runs `mockery` recursively via `.mockery.yml` and then `go generate ./...`.
- `oms beta bootstrap-local` is for testing, not production.
- Local bootstrap is Linux/VM oriented, not macOS Minikube. It expects Helm 3+, `sops`, `age-keygen`, and registry credentials.
