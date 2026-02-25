// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type BootstrapGcpPostconfigCmd struct {
	cmd *cobra.Command

	Opts          *BootstrapGcpPostconfigOpts
	CodesphereEnv gcp.CodesphereEnvironment
}

type BootstrapGcpPostconfigOpts struct {
	*GlobalOptions
	InstallConfigPath string
	PrivateKeyPath    string
}

func (c *BootstrapGcpPostconfigCmd) RunE(_ *cobra.Command, args []string) error {
	log.Printf("running post-configuration steps...")

	icg := installer.NewInstallConfigManager()

	fw := util.NewFilesystemWriter()

	envFileContent, err := fw.ReadFile(gcp.GetInfraFilePath())
	if err != nil {
		return fmt.Errorf("failed to read gcp infra file: %w", err)
	}

	err = json.Unmarshal(envFileContent, &c.CodesphereEnv)
	if err != nil {
		return fmt.Errorf("failed to unmarshal gcp infra file: %w", err)
	}

	err = icg.LoadInstallConfigFromFile(c.Opts.InstallConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	return fmt.Errorf("not implemented: run config script on k0s-1 node to install GCP CCM")
}

func AddBootstrapGcpPostconfigCmd(bootstrapGcp *cobra.Command, opts *GlobalOptions) {
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
	flags.StringVar(&postconfig.Opts.PrivateKeyPath, "private-key-path", "", "Path to the GCP service account private key file (optional)")

	bootstrapGcp.AddCommand(postconfig.cmd)
	postconfig.cmd.RunE = postconfig.RunE
}
