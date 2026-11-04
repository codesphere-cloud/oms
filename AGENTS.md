# Repository guidance

## Required checks

- Run focused package tests during development.
- Both `make test` and `make lint` must pass before completing Go code or test changes. Also run `make lint-changed` to check changes against `origin/main`. Fix failures before marking the work complete.

## Architecture and CLI conventions

- Wrap external dependencies behind small interfaces as much as practical, with concrete production implementations.
- Keep Cobra registration focused on command metadata and flags, and command execution focused on validation and orchestration. Keep reusable installation, bootstrap, and client logic in the corresponding `internal/` packages.
- Follow the existing command structure under `cli/cmd/` and its subpackages. Use `cli/cmd/util.GlobalOptions` for global options and `util.AddCmd` for command registration where applicable.
- Generally use verb-first commands and keep presentation in the command layer.
- Wrap errors with useful context and `%w`.
- Include the existing copyright/SPDX header in Go files and format changes with `gofmt`.

## Testing

- Write all new and modified Go tests with Ginkgo v2 and Gomega. Use the existing suite setup and prefer external test packages. The standard `testing` package may be used only for the `RunSpecs` suite entry point.
- Write individual Ginkgo `It` cases with Gomega matchers. Do not use table-driven tests (`DescribeTable`, `Entry`, or loops over test cases).
- Use Mockery-generated mocks for external dependencies; do not hand-write dependency stubs or fakes.
- Wrap as many external dependencies as practical when writing tests. Avoid tests that write files or depend on elapsed time.
- Keep unit tests independent of live APIs, cloud infrastructure, and clusters. Keep integration tests separate using the existing `integration` build tag.

## Generated files

- After changing mocked interfaces, regenerate mocks with `go tool mockery`, configured in `.mockery.yml`. Do not manually edit generated mocks.
- Docs and licenses are auto-generated on PRs; no need to generate them locally.

## Code quality and review

- Keep control flow flat and complexity low. Prefer early returns and extract coherent validation or branching logic into clearly named subfunctions instead of deeply nested conditionals.
- Avoid unnecessary `else` branches after a branch returns. When a multi-branch conditional remains difficult to follow, move it into a subfunction that returns a result or error so the caller stays linear.
- Represent replaceable behavior and external dependencies with small interfaces and concrete production implementations. Do not pass collections of callback functions through commands or services; generate interface mocks with mockery for tests.
- Keep tests proportional to realistic contracts and failure modes. Cover meaningful boundaries and plausible failures, but avoid over-pessimistic cases for states already made impossible by types, validated inputs, or guaranteed lower-layer behavior. Document why a defensive case is realistic when that is not obvious.

## Pull requests

- Open pull requests with Conventional Commit titles so they trigger the automatic release flows. Use `update`, `refactor`, `fix`, or `feat` as the type, in the format `<type>: <description>` or `<type>(<scope>): <description>`.
