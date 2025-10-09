# Contributing to Codesphere OMS

We welcome contributions of all kinds! By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Report Issues

If you encounter a bug or have a feature request, please [open a new issue](https://github.com/codesphere-cloud/oms/issues/new) on GitHub. Please include the following information:

* **Operating System and Version:**
* **OMS Version (if applicable):**
* **Steps to Reproduce the Bug:**
* **Expected Behavior:**
* **Actual Behavior:**
* **Any relevant logs or error messages:**

## How to Suggest Features or Improvements

We'd love to hear your ideas! Please [open a new issue](https://github.com/codesphere-cloud/oms/issues/new) to discuss your proposed feature or improvement before submitting code. This allows us to align on the design and approach.

## How to Add a New Subcommand to the CLI

This project currently uses a fork of cobra-cli with locally-scoped variables: https://github.com/NautiluX/cobra-cli-local.

Please use it to add new commands to the OMS CLI like following:

```
cobra-cli add --copyright=false -L -d cli -p install component
```

Run the generated `AddInstallComponent()` function in the parent `cli/cmd/install.go` to add the subcommand.
This will add the following command to the CLI:

```
oms-cli install component
```

## Contributing Code

If you'd like to contribute code, please follow these steps:

1.  **Fork the Repository:** Fork this repository to your GitHub account.
2.  **Create a Branch:** Create a new branch for your changes: `git checkout -b feature/your-feature-name`
3.  **Set Up Development Environment:**

    * Ensure you have Go installed. The minimum required Go version is specified in the `go.mod` file.
    * Clone your forked repository: `git clone git@github.com:your-username/oms.git`
    * Navigate to the project directory: `cd oms`
    * Run `make`: This command should download necessary dependencies and build the CLI.

4.  **Follow Coding Standards:**

    * Please ensure your code is properly formatted using `go fmt`.
    * We use [golangci-lint](https://golangci-lint.run/) for static code analysis. Please run it locally before submitting a pull request: `make lint`.

5.  **Write Tests:**

    * We use [Ginkgo](https://github.com/onsi/ginkgo) and [Gomega](https://github.com/onsi/gomega) for testing.
    * Please write tests for your code using Ginkgo and Gomega and add them to the `_test.go` files.
    * Aim for good test coverage.

6.  **Build and Test:**

    * Ensure everything is working correctly by running the appropriate `make` targets (e.g., `make build`, `make test`). The `make test` target should run the Ginkgo tests.

7.  **Commit Your Changes:**

    * We use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) for our commit messages. Please format your commit messages according to the Conventional Commits specification. Examples:
        * `fix(api): Handle edge case in API client`
        * `feat(cli): Add new command for listing resources`
        * `docs: Update contributing guide with commit message conventions`
    * **Developer Certificate of Origin (DCO)**

        In order to contribute to this project, you must agree to the [Developer Certificate of Origin (DCO)](https://developercertificate.org/). This is a simple statement that you, as a contributor, have the right to submit the code you are contributing.

        ```text
        Developer's Certificate of Origin 1.1

        By making a contribution to this project, I certify that:

        (a) The contribution was created in whole or in part by me and I
            have the right to submit it under the open source license
            indicated in the file; or

        (b) The contribution is based upon previous work that, to the best
            of my knowledge, is covered under an appropriate open source
            license and I have the right under that license to submit that
            work with modifications, whether created in whole or in part
            by me, or solely by me; or

        (c) The contribution was provided directly to me by some other
            person who certified (a), (b) or (c) and I have not modified
            it.

        (d) I understand and agree that this project and the contribution
            are public and that a record of the contribution (including all
            personal information I submit with it) is maintained indefinitely
            and may be redistributed consistent with this project or the
            open source license(s) involved.
        ```

        To indicate that you accept the DCO, you must add a `Signed-off-by` line to each of your commit messages. Here's an example:

        ```
        Fix: Handle edge case in API client

        This commit fixes a bug where the API client would crash when receiving
        an empty response.

        Signed-off-by: John Doe <john.doe@example.com>
        ```

        You can add this line to your commit message using the `-s` flag with the `git commit` command:

        ```bash
        git commit -s -m "Your commit message"
        ```

8.  **Submit a Pull Request:** [Open a new pull request](https://github.com/codesphere-cloud/oms/compare) to the `main` branch of this repository. Please include a clear description of your changes and reference any related issues.

## Code Review Process

All contributions will be reviewed by project maintainers. Please be patient during the review process and be prepared to make revisions based on feedback. We aim for thorough but timely reviews.

## License

By contributing to Codesphere OMS, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

## Community

Connect with the community and ask questions by joining our mailing list: [oms@codesphere.com](mailto:oms@codesphere.com).

Thank you for your interest in contributing to Codesphere OMS!
