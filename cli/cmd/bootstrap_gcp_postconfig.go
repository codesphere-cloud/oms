// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type BootstrapGcpPostconfigCmd struct {
	cmd *cobra.Command

	Opts          *BootstrapGcpPostconfigOpts
	CodesphereEnv gcp.CodesphereEnvironment
}

type BootstrapGcpPostconfigOpts struct {
	*util.GlobalOptions
	InstallConfigPath string
	SecretsFilePath   string
	AgeKey            string
	PrivateKeyPath    string
}

func (c *BootstrapGcpPostconfigCmd) RunE(_ *cobra.Command, args []string) error {
	log.Printf("running post-configuration steps...")

	fw := intutil.NewFilesystemWriter()

	_, ageKey, err := resolveVaultAccess(fw, c.Opts.SecretsFilePath, c.Opts.AgeKey)
	if err != nil {
		return err
	}

	icg, err := installer.NewInstallConfigManager(string(vault.TypeAuto), ageKey)
	if err != nil {
		return fmt.Errorf("failed to initialize config manager: %w", err)
	}

	if err := icg.LoadVaultFromFileOrCreate(c.Opts.SecretsFilePath); err != nil {
		return fmt.Errorf("failed to load vault file: %w", err)
	}

	infraFilePath := gcp.GetInfraFilePath()

	codesphereEnv, exists, err := gcp.LoadInfraFile(fw, infraFilePath)
	if err != nil {
		return fmt.Errorf("failed to load gcp infra file: %w", err)
	}

	if !exists {
		return fmt.Errorf("gcp infra file not found at %s", infraFilePath)
	}

	c.CodesphereEnv = codesphereEnv

	err = icg.LoadInstallConfigFromFile(c.Opts.InstallConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	return fmt.Errorf("not implemented: run config script on k0s-1 node to install GCP CCM")
}

func AddBootstrapGcpPostconfigCmd(bootstrapGcp *cobra.Command, opts *util.GlobalOptions) {
	postconfig := BootstrapGcpPostconfigCmd{
		cmd: &cobra.Command{
			Use:   "postconfig",
			Short: "Run post-configuration steps for GCP bootstrapping",
			Long: io.Long(`After bootstrapping GCP infrastructure, this command runs additional configuration steps
							to finalize the setup for the Codesphere cluster on GCP:

							* Install Google Cloud Controller Manager for ingress management.`),
		},
		Opts: &BootstrapGcpPostconfigOpts{
			GlobalOptions: opts,
		},
	}

	flags := postconfig.cmd.Flags()
	flags.StringVar(&postconfig.Opts.InstallConfigPath, "install-config-path", "config.yaml", "Path to the installation configuration file")
	flags.StringVar(&postconfig.Opts.SecretsFilePath, "secrets-file", "prod.vault.yaml", "Path to the secrets (vault) file")
	flags.StringVar(&postconfig.Opts.AgeKey, "age-key", "", "Path to the age private key (required for sops unless SOPS_AGE_KEY or SOPS_AGE_KEY_FILE is set)")
	flags.StringVar(&postconfig.Opts.PrivateKeyPath, "private-key-path", "", "Path to the GCP service account private key file (optional)")

	util.AddCmd(bootstrapGcp, postconfig.cmd)
	postconfig.cmd.RunE = postconfig.RunE
}
