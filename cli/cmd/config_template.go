// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type ConfigTemplateCmd struct {
	cmd  *cobra.Command
	Opts ConfigTemplateOpts
}

type ConfigTemplateOpts struct {
	*GlobalOptions
	Config string
	Vault  string
	AgeKey string
}

func (c *ConfigTemplateCmd) RunE(cmd *cobra.Command, _ []string) error {
	rendered, err := c.Render()
	if err != nil {
		return err
	}

	if _, err := fmt.Fprint(cmd.OutOrStdout(), string(rendered)); err != nil {
		return fmt.Errorf("failed to write rendered config: %w", err)
	}

	return nil
}

func AddConfigTemplateCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	templateCmd := &ConfigTemplateCmd{
		cmd: &cobra.Command{
			Use:   "template",
			Short: "Render a config.yaml template using secrets from a vault file",
			Long: io.Long(`Render a config.yaml template using secrets from a prod.vault.yaml file.

This command prints the rendered configuration to stdout so templating can be tested without running an installation.`),
			Example: formatExamples("config template", []io.Example{
				{
					Cmd:  "--config config.yaml --vault prod.vault.yaml --age-key age_key.txt",
					Desc: "Render config.yaml with secrets from prod.vault.yaml",
				},
			}),
			Args: cobra.ExactArgs(0),
		},
		Opts: ConfigTemplateOpts{GlobalOptions: opts},
	}

	templateCmd.cmd.Flags().StringVarP(&templateCmd.Opts.Config, "config", "c", "", "Path to the config.yaml template to render (required)")
	templateCmd.cmd.Flags().StringVarP(&templateCmd.Opts.Vault, "vault", "v", "", "Path to the SOPS-encrypted prod.vault.yaml file (required)")
	templateCmd.cmd.Flags().StringVarP(&templateCmd.Opts.AgeKey, "age-key", "k", "", "Path to the age key file used to decrypt the vault (required)")

	util.MarkFlagRequired(templateCmd.cmd, "config")
	util.MarkFlagRequired(templateCmd.cmd, "vault")
	util.MarkFlagRequired(templateCmd.cmd, "age-key")

	AddCmd(parentCmd, templateCmd.cmd)

	templateCmd.cmd.RunE = templateCmd.RunE
}

func (c *ConfigTemplateCmd) Render() ([]byte, error) {
	data, err := os.ReadFile(c.Opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", c.Opts.Config, err)
	}

	store := installer.NewLazyVaultTemplatingSecretStore(c.Opts.Vault, c.Opts.AgeKey)
	rendered, err := configtemplating.RenderInstallConfigTemplate(data, store)
	if err != nil {
		return nil, fmt.Errorf("failed to render config template: %w", err)
	}

	return rendered, nil
}
