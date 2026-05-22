// Copyright (c) Codesphere Inc. SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"

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
	Name       string
	URL        string
	RepoName   string
	Type       string
	Username   string
	EnableOCI  bool
	SecretType string
}

func (c *InstallArgoCDRepoSecretCmd) RunE(_ *cobra.Command, args []string) error {
	requiredFlags := map[string]string{
		"name":      c.Opts.Name,
		"url":       c.Opts.URL,
		"repo-name": c.Opts.RepoName,
		"type":      c.Opts.Type,
		"username":  c.Opts.Username,
	}

	for flagName, value := range requiredFlags {
		if value == "" {
			return fmt.Errorf("flag --%s is required", flagName)
		}
	}

	if c.Opts.Type != "helm" && c.Opts.Type != "git" {
		return fmt.Errorf("flag --type must be either \"helm\" or \"git\", got %q", c.Opts.Type)
	}

	password, err := c.resolvePassword()
	if err != nil {
		return err
	}

	cfg := installer.ArgoCDRepoSecretConfig{
		Name:       c.Opts.Name,
		URL:        c.Opts.URL,
		RepoName:   c.Opts.RepoName,
		Type:       c.Opts.Type,
		Username:   c.Opts.Username,
		Password:   password,
		EnableOCI:  c.Opts.EnableOCI,
		SecretType: c.Opts.SecretType,
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
// or prompts the user interactively if the env var is not set.
func (c *InstallArgoCDRepoSecretCmd) resolvePassword() (string, error) {
	if pw := os.Getenv("OMS_REPO_PASSWORD"); len(pw) != 0 {
		return pw, nil
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
			Short: "Create or update an ArgoCD repository secret",
			Long: packageio.Long(`Create or update an ArgoCD repository secret for authenticating against
				Helm OCI registries or Git repositories.

				The password is read from the OMS_REPO_PASSWORD environment variable.
				If not set, it will be prompted interactively (hidden input).
				You can also pipe the password via stdin: echo "token" | oms beta install argocd-repo-secret ...`),
			Example: formatExamples("install argocd-repo-secret", []packageio.Example{
				{Cmd: "--name ghcr-codesphere-helm-repo --url ghcr.io/codesphere-cloud/charts --repo-name ghcr-codesphere --type helm --username CodesphereBot --enable-oci", Desc: "Create a Helm OCI registry secret (prompts for password)"},
				{Cmd: "--name my-git-repo --url https://github.com/my-org --repo-name my-org --type git --username bot --secret-type repo-creds", Desc: "Create a git repo credentials secret (set OMS_REPO_PASSWORD env var beforehand)"},
			}),
		},
	}
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Name, "name", "", "Name of the Kubernetes Secret (metadata.name)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.URL, "url", "", "Repository URL (e.g. ghcr.io/codesphere-cloud/charts)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.RepoName, "repo-name", "", "Display name for the repository in ArgoCD")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Type, "type", "", "Repository type: \"helm\" or \"git\"")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Username, "username", "", "Username for repository authentication")
	repoSecret.cmd.Flags().BoolVar(&repoSecret.Opts.EnableOCI, "enable-oci", false, "Enable OCI support (sets enableOCI: \"true\" in the secret)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.SecretType, "secret-type", "repository", "ArgoCD secret type label value (\"repository\" or \"repo-creds\")")
	repoSecret.cmd.RunE = repoSecret.RunE

	AddCmd(parentCmd, repoSecret.cmd)
}
