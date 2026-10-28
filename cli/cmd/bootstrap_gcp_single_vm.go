// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/bootstrap/local"
	"github.com/spf13/pflag"
)

func (c *BootstrapGcpCmd) addBootstrapModeFlags() {
	f := c.cmd.Flags()
	f.BoolVar(&c.CodesphereEnv.SingleVM, "single-vm", false, "Install k0s and Codesphere on one GCP VM using the local bootstrap")
	f.StringVar(&c.CodesphereEnv.InstallConfigTemplatePath, "install-config-template", "", "Initial config.yaml template, applied before --config files")
	f.StringArrayVar(&c.CodesphereEnv.InstallConfigs, "config", nil, "Local config.yaml file; repeat to merge in order before VM settings")
	f.StringVar(&c.CodesphereEnv.InstallVaultPath, "vault", "", "SOPS vault for config templating and initial install secrets")
	f.StringVar(&c.CodesphereEnv.InstallVaultPrivKey, "priv-key", "", "Age private key for --vault (or use the age key environment variable)")
	f.StringVar(&c.CodesphereEnv.MachineType, "machine-type", "", "GCP machine type override for all VMs (default: role-specific sizes; e2-standard-8 for a single VM)")
	f.StringVar(&c.StorageEngine, "storage-engine", local.StorageEngineRookCeph, "Single VM: storage engine (rook-ceph or local)")
}

// prepareBootstrapOptions validates mode-specific flags before creating external clients.
func (c *BootstrapGcpCmd) prepareBootstrapOptions() error {
	if c.CodesphereEnv.GitHubPAT == "" {
		c.CodesphereEnv.GitHubPAT = os.Getenv("OMS_REGISTRY_PASSWORD")
	}

	if c.CodesphereEnv.InstallVaultPrivKey != "" && c.CodesphereEnv.InstallVaultPath == "" {
		return fmt.Errorf("--priv-key requires --vault")
	}

	if !c.CodesphereEnv.SingleVM {
		return c.validateMultiVMFlags()
	}

	if err := c.validateSingleVMFlags(); err != nil {
		return err
	}

	if err := validateLocalStorageEngine(c.StorageEngine); err != nil {
		return err
	}

	if c.CodesphereEnv.RegistryUser == "" {
		return fmt.Errorf("--registry-user is required with --single-vm")
	}

	if c.CodesphereEnv.GitHubPAT == "" {
		return fmt.Errorf("--github-pat or OMS_REGISTRY_PASSWORD must be set for the local Codesphere installer")
	}

	return nil
}

func (c *BootstrapGcpCmd) validateMultiVMFlags() error {
	for _, name := range []string{"storage-engine"} {
		if c.cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s requires --single-vm", name)
		}
	}

	return nil
}

func (c *BootstrapGcpCmd) validateSingleVMFlags() error {
	var flagErr error

	c.cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flagErr != nil || !flag.Changed || singleVMSupportsFlag(flag.Name) {
			return
		}

		flagErr = fmt.Errorf("--%s is not supported with --single-vm; supply installation settings through --config and --vault", flag.Name)
	})

	return flagErr
}

// Only flags consumed by the single VM workflow are accepted, so multi-host
// installation options cannot be silently ignored by the local installer.
func singleVMSupportsFlag(name string) bool {
	switch name {
	case "single-vm", "project-name", "project-ttl", "billing-account", "folder-id",
		"base-domain", "dns-project-id", "dns-zone-name", "region", "zone",
		"ssh-public-key-path", "ssh-private-key-path", "install-local", "install-version",
		"install-hash", "remote-oms-binary", "registry-user", "github-pat", "spot-vms",
		"root-disk-size", "ssh-quiet", "timer", "install-config-template", "config",
		"vault", "priv-key", "machine-type", "os", "storage-engine", "help":
		return true
	default:
		return false
	}
}

func (c *BootstrapGcpCmd) bootstrapSingleVM(bs *gcp.GCPBootstrapper) error {
	if err := bs.BootstrapSingleVM(c.CodesphereEnv.RegistryUser, c.StorageEngine, c.StepTimer); err != nil {
		return fmt.Errorf("failed to bootstrap single VM: %w", err)
	}

	log.Printf("Codesphere installed on %s (ssh root@%s)", bs.Env.Jumpbox.GetName(), bs.Env.Jumpbox.GetExternalIP())
	log.Printf("Codesphere URL: https://cs.%s", bs.Env.BaseDomain)

	return nil
}
