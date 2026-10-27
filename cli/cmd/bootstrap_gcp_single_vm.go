// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/spf13/pflag"
)

// prepareBootstrapOptions validates mode-specific flags before creating external clients.
func (c *BootstrapGcpCmd) prepareBootstrapOptions() error {
	if c.CodesphereEnv.GitHubPAT == "" {
		c.CodesphereEnv.GitHubPAT = os.Getenv("OMS_REGISTRY_PASSWORD")
	}

	if c.CodesphereEnv.InstallVaultPrivKey != "" && c.CodesphereEnv.InstallVaultPath == "" {
		return fmt.Errorf("--priv-key requires --vault")
	}

	if err := c.validateLegacyBootstrapFlags(); err != nil {
		return err
	}

	if !c.CodesphereEnv.SingleVM {
		return nil
	}

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

// validateLegacyBootstrapFlags uses the subcommand's registered options as the
// source of truth for the original flag-based entry point.
func (c *BootstrapGcpCmd) validateLegacyBootstrapFlags() error {
	if !c.legacyMode {
		return nil
	}

	if !c.CodesphereEnv.SingleVM {
		if c.cmd.Flags().Changed("storage-engine") {
			return fmt.Errorf("--storage-engine requires --single-vm")
		}

		return nil
	}

	supported := newBootstrapGcpSingleNodeCmd(c.Opts).cmd.Flags()

	var flagErr error

	c.cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flagErr != nil || !flag.Changed || flag.Name == "single-vm" || flag.Name == "help" || supported.Lookup(flag.Name) != nil {
			return
		}

		flagErr = fmt.Errorf("--%s is not supported with --single-vm; supply installation settings through --config and --vault", flag.Name)
	})

	return flagErr
}

func (c *BootstrapGcpCmd) bootstrapSingleVM(bs *gcp.GCPBootstrapper) error {
	if err := bs.BootstrapSingleVM(c.CodesphereEnv.RegistryUser, c.StorageEngine, c.StepTimer); err != nil {
		return fmt.Errorf("failed to bootstrap single VM: %w", err)
	}

	log.Printf("Codesphere installed on %s (ssh root@%s)", bs.Env.Jumpbox.GetName(), bs.Env.Jumpbox.GetExternalIP())
	log.Printf("Codesphere URL: https://cs.%s", bs.Env.BaseDomain)

	return nil
}
