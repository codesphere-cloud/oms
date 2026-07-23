// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/spf13/cobra"
)

type TemplateConfigCmd struct {
	cmd  *cobra.Command
	Opts TemplateConfigOpts
}

type TemplateConfigOpts struct {
	*util.GlobalOptions
	Config string
	Vault  string
	AgeKey string
}

func (c *TemplateConfigCmd) RunE(cmd *cobra.Command, _ []string) error {
	rendered, err := c.Render()
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: the rendered config is printed to stdout in plaintext and may contain secrets from the vault."); err != nil {
		return fmt.Errorf("failed to write warning message: %w", err)
	}

	if _, err := fmt.Fprint(cmd.OutOrStdout(), string(rendered)); err != nil {
		return fmt.Errorf("failed to write rendered config: %w", err)
	}

	return nil
}

func AddTemplateConfigCmd(parentCmd *cobra.Command, opts *util.GlobalOptions) {
	configCmd := &TemplateConfigCmd{
		cmd: &cobra.Command{
			Use:   "config",
			Short: "Render a config.yaml template using secrets from a vault file",
			Long: io.Long(`Render a config.yaml template using secrets from a prod.vault.yaml file.

This command prints the rendered configuration to stdout so templating can be tested without running an installation.

Template syntax in config.yaml:

  # Inject a secret value (defaults to the "content"/"password" field)
  someKey: "{{ secret "mySecret" }}"

  # Select a specific field
  username: "{{ secret "mySecret" "fields.username" }}"
  password: "{{ secret "mySecret" "fields.password" }}"

  # Inject a file secret's content
  caCert: "{{ secret "caCert" "file.content" }}"

Secret names and selectors must match entries in the prod.vault.yaml file.`),
			Example: util.FormatExamples("template config", []io.Example{
				{
					Cmd:  "--config config.yaml --vault prod.vault.yaml --age-key age_key.txt",
					Desc: "Render config.yaml with secrets from prod.vault.yaml",
				},
			}),
			Args: cobra.ExactArgs(0),
		},
		Opts: TemplateConfigOpts{GlobalOptions: opts},
	}

	configCmd.cmd.Flags().StringVarP(&configCmd.Opts.Config, "config", "c", "", "Path to the config.yaml template to render (required)")
	configCmd.cmd.Flags().StringVarP(&configCmd.Opts.Vault, "vault", "v", "", "Path to the SOPS-encrypted prod.vault.yaml file (required)")
	configCmd.cmd.Flags().StringVarP(&configCmd.Opts.AgeKey, "age-key", "k", "", "Path to the age key file used to decrypt the vault (required)")

	util.MarkFlagRequired(configCmd.cmd, "config")
	util.MarkFlagRequired(configCmd.cmd, "vault")
	util.MarkFlagRequired(configCmd.cmd, "age-key")

	util.AddCmd(parentCmd, configCmd.cmd)

	configCmd.cmd.RunE = configCmd.RunE
}

func (c *TemplateConfigCmd) Render() ([]byte, error) {
	data, err := os.ReadFile(c.Opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", c.Opts.Config, err)
	}

	store := vault.NewLazyVaultTemplatingSecretStore(c.Opts.Vault, c.Opts.AgeKey)
	rendered, err := configtemplating.RenderInstallConfigTemplate(data, store)
	if err != nil {
		return nil, fmt.Errorf("failed to render config template: %w", err)
	}

	return rendered, nil
}
