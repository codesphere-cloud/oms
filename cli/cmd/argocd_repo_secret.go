// Copyright (c) Codesphere Inc. SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
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
	Password   string
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
		"password":  c.Opts.Password,
	}

	for flagName, value := range requiredFlags {
		if value == "" {
			return fmt.Errorf("flag --%s is required", flagName)
		}
	}

	if c.Opts.Type != "helm" && c.Opts.Type != "git" {
		return fmt.Errorf("flag --type must be either \"helm\" or \"git\", got %q", c.Opts.Type)
	}

	cfg := installer.ArgoCDRepoSecretConfig{
		Name:       c.Opts.Name,
		URL:        c.Opts.URL,
		RepoName:   c.Opts.RepoName,
		Type:       c.Opts.Type,
		Username:   c.Opts.Username,
		Password:   c.Opts.Password,
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

func AddArgoCDRepoSecretCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	repoSecret := InstallArgoCDRepoSecretCmd{
		cmd: &cobra.Command{
			Use:   "argocd-repo-secret",
			Short: "Create or update an ArgoCD repository secret",
			Long:  packageio.Long(`Create or update an ArgoCD repository secret for authenticating against Helm OCI registries or Git repositories.`),
			Example: formatExamples("install argocd-repo-secret", []packageio.Example{
				{Cmd: "--name ghcr-codesphere-helm-repo --url ghcr.io/codesphere-cloud/charts --repo-name ghcr-codesphere --type helm --username CodesphereBot --password <token> --enable-oci", Desc: "Create a Helm OCI registry secret"},
				{Cmd: "--name my-git-repo --url https://github.com/my-org --repo-name my-org --type git --username bot --password <token> --secret-type repo-creds", Desc: "Create a git repo credentials secret"},
			}),
		},
	}
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Name, "name", "", "Name of the Kubernetes Secret (metadata.name)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.URL, "url", "", "Repository URL (e.g. ghcr.io/codesphere-cloud/charts)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.RepoName, "repo-name", "", "Display name for the repository in ArgoCD")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Type, "type", "", "Repository type: \"helm\" or \"git\"")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Username, "username", "", "Username for repository authentication")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.Password, "password", "", "Password or token for repository authentication")
	repoSecret.cmd.Flags().BoolVar(&repoSecret.Opts.EnableOCI, "enable-oci", false, "Enable OCI support (sets enableOCI: \"true\" in the secret)")
	repoSecret.cmd.Flags().StringVar(&repoSecret.Opts.SecretType, "secret-type", "repository", "ArgoCD secret type label value (\"repository\" or \"repo-creds\")")
	repoSecret.cmd.RunE = repoSecret.RunE

	AddCmd(parentCmd, repoSecret.cmd)
}
