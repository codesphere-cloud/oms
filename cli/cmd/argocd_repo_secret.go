// Copyright (c) Codesphere Inc. SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// InstallArgoCDRepoSecretCmd represents the argocd-repo-secret command
type InstallArgoCDRepoSecretCmd struct {
	cmd  *cobra.Command
	Opts InstallArgoCDRepoSecretOpts
}

type InstallArgoCDRepoSecretOpts struct {
	*GlobalOptions
	URL      string
	Username string
}

func (c *InstallArgoCDRepoSecretCmd) RunE(_ *cobra.Command, args []string) error {
	password, err := c.resolvePassword()
	if err != nil {
		return err
	}

	cfg := installer.ArgoCDRepoSecretConfig{
		Name:       "codesphere-helm-repo",
		URL:        c.Opts.URL,
		RepoName:   "codesphere-helm-repo",
		Type:       "helm",
		Username:   c.Opts.Username,
		Password:   password,
		EnableOCI:  true,
		SecretType: "repository",
	}

	repoSecret, err := installer.NewArgoCDRepoSecret(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize ArgoCD repo secret installer: %w", err)
	}

	if err := repoSecret.Apply(context.Background()); err != nil {
		return fmt.Errorf("failed to apply ArgoCD repo secret: %w", err)
	}

	return nil
}

// resolvePassword reads the password from the OMS_REPO_PASSWORD environment variable,
// from stdin if piped, or prompts the user interactively if neither is available.
func (c *InstallArgoCDRepoSecretCmd) resolvePassword() (string, error) {
	if pw := os.Getenv("OMS_REPO_PASSWORD"); len(pw) != 0 {
		return pw, nil
	}

	// If stdin is not a terminal, read the password from the pipe.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			pw := strings.TrimSpace(scanner.Text())
			if len(pw) == 0 {
				return "", fmt.Errorf("password is required; received empty input from stdin")
			}
			return pw, nil
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to read password from stdin: %w", err)
		}
		return "", fmt.Errorf("password is required; no input received from stdin")
	}

	fmt.Print("Repository password/token: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	if len(pw) == 0 {
		return "", fmt.Errorf("password is required; set OMS_REPO_PASSWORD or enter it when prompted")
	}
	return string(pw), nil
}

func AddArgoCDRepoSecretCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	repoSecret := InstallArgoCDRepoSecretCmd{
		cmd: &cobra.Command{
			Use:   "argocd-repo-secret",
			Short: "Create or update the Codesphere Helm repository secret in ArgoCD",
			Long: packageio.Long(`Create or update the ArgoCD repository secret for authenticating against
				the Codesphere Helm chart OCI registry.

				Use --url to point to a mirror of the registry if needed.

				The password is read from the OMS_REPO_PASSWORD environment variable.
				If not set, it will be prompted interactively (hidden input).
				You can also pipe the password via stdin: echo "token" | oms beta install argocd-repo-secret ...`),
			Example: formatExamples("beta install argocd-repo-secret", []packageio.Example{
				{Cmd: "", Desc: "Create the secret using defaults (prompts for password)"},
				{Cmd: "--url my-mirror.example.com/charts", Desc: "Use a mirrored registry URL"},
				{Cmd: "--url my-mirror.example.com/charts --username MyBot", Desc: "Use a mirrored registry with custom username"},
			}),
		},
	}
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.URL, "url", "ghcr.io/codesphere-cloud/charts", "Helm OCI registry URL (customize for mirrors)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Username, "username", "CodesphereBot", "Username for registry authentication")
	repoSecret.cmd.RunE = repoSecret.RunE

	AddCmd(parentCmd, repoSecret.cmd)
}
