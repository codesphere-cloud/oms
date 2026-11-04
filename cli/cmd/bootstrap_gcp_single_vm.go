// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/bootstrap/local"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/spf13/cobra"
)

// AddBootstrapGcpSingleNodeCmd registers the single-node bootstrap subcommand.
func AddBootstrapGcpSingleNodeCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	util.AddCmd(parent, newBootstrapGcpSingleNodeCmd(opts).cmd)
}

func newBootstrapGcpSingleNodeCmd(opts *util.GlobalOptions) *BootstrapGcpCmd {
	c := &BootstrapGcpCmd{
		cmd: &cobra.Command{
			Use:   "single-node",
			Short: "Bootstrap k0s and Codesphere on one GCP VM",
			Long: io.Long(`Install k0s and Codesphere on one GCP VM using the local bootstrap.
				Requires --registry-user, either --github-pat or OMS_REGISTRY_PASSWORD,
				and either --install-local or --install-version. The root disk defaults to 50 GB;
				a separate 100 GB Ceph disk is added. Supply installation settings through --config and --vault.
				Not for production use.`),
			Args: cobra.NoArgs,
		},
		Opts:          opts,
		Env:           env.NewEnv(),
		CodesphereEnv: &gcp.CodesphereEnvironment{SingleVM: true},
	}
	c.cmd.RunE = func(command *cobra.Command, args []string) error {
		if err := c.prepareBootstrapOptions(); err != nil {
			return err
		}

		if err := c.validateSingleVMOptions(); err != nil {
			return err
		}

		return c.RunE(command, args)
	}
	c.addSharedBootstrapFlags()
	c.cmd.Flags().StringVar(&c.StorageEngine, "storage-engine", local.StorageEngineRookCeph, "Storage engine (rook-ceph or local)")

	return c
}

func (c *BootstrapGcpCmd) validateSingleVMOptions() error {
	if err := validateLocalStorageEngine(c.StorageEngine); err != nil {
		return err
	}

	if c.CodesphereEnv.RegistryUser == "" {
		return fmt.Errorf("--registry-user is required for single-node")
	}

	if c.CodesphereEnv.GitHubPAT == "" {
		return fmt.Errorf("--github-pat or OMS_REGISTRY_PASSWORD must be set for the local Codesphere installer")
	}

	return nil
}

func (c *BootstrapGcpCmd) bootstrapSingleVM(bs *gcp.GCPBootstrapper) error {
	if err := bs.BootstrapSingleVM(c.CodesphereEnv.RegistryUser, c.StorageEngine, c.StepTimer); err != nil {
		return fmt.Errorf("failed to bootstrap single VM: %w", err)
	}

	log.Printf("Codesphere installed on %s (ssh root@%s)", bs.Env.Jumpbox.GetName(), bs.Env.Jumpbox.GetExternalIP())
	log.Printf("Codesphere URL: https://cs.%s", bs.Env.BaseDomain)

	return nil
}
