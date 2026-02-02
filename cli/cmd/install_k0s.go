// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"path/filepath"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// InstallK0sCmd represents the k0s download command
type InstallK0sCmd struct {
	cmd        *cobra.Command
	Opts       InstallK0sOpts
	Env        env.Env
	FileWriter util.FileIO
}

type InstallK0sOpts struct {
	*GlobalOptions
	Version       string
	K0sctlVersion string
	Package       string
	InstallConfig string
	SSHKeyPath    string
	Force         bool
	NoDownload    bool
}

func (c *InstallK0sCmd) RunE(_ *cobra.Command, args []string) error {
	hw := portal.NewHttpWrapper()
	env := c.Env
	pm := installer.NewPackage(env.GetOmsWorkdir(), c.Opts.Package)
	k0s := installer.NewK0s(hw, env, c.FileWriter)
	k0sctl := installer.NewK0sctl(hw, env, c.FileWriter)

	return c.InstallK0s(pm, k0s, k0sctl)
}

func AddInstallK0sCmd(install *cobra.Command, opts *GlobalOptions) {
	k0s := InstallK0sCmd{
		cmd: &cobra.Command{
			Use:   "k0s",
			Short: "Install k0s Kubernetes distribution",
			Long: packageio.Long(`Install k0s either from the package or by downloading it.
			This command uses k0sctl to deploy k0s clusters from a Codesphere install-config.
			
			You must provide a Codesphere install-config file, which will:
			- Generate a k0s configuration from the install-config
			- Generate a k0sctl configuration for cluster deployment
			- Deploy k0s to all nodes defined in the install-config using k0sctl`),
			Example: formatExamplesWithBinary("install k0s", []packageio.Example{
				{Cmd: "--install-config <path>", Desc: "Path to Codesphere install-config file to generate k0s config from"},
				{Cmd: "--version <version>", Desc: "Version of k0s to install (e.g., v1.30.0+k0s.0)"},
				{Cmd: "--k0sctl-version <version>", Desc: "Version of k0sctl to use (e.g., v0.17.4)"},
				{Cmd: "--package <file>", Desc: "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from"},
				{Cmd: "--ssh-key-path <path>", Desc: "SSH private key path for remote installation"},
				{Cmd: "--force", Desc: "Force new download and installation"},
				{Cmd: "--no-download", Desc: "Skip downloading k0s binary (expects it to be on remote nodes)"},
			}, "oms-cli"),
		},
		Opts:       InstallK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", "", "Version of k0s to install")
	k0s.cmd.Flags().StringVar(&k0s.Opts.K0sctlVersion, "k0sctl-version", "", "Version of k0sctl to use")
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from")
	k0s.cmd.Flags().StringVar(&k0s.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file (required)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path for remote installation")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force new download and installation")
	k0s.cmd.Flags().BoolVar(&k0s.Opts.NoDownload, "no-download", false, "Skip downloading k0s binary")

	_ = k0s.cmd.MarkFlagRequired("install-config")

	install.AddCommand(k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

const (
	defaultK0sPath   = "kubernetes/files/k0s"
	k0sctlConfigFile = "k0sctl-config.yaml"
)

func (c *InstallK0sCmd) InstallK0s(pm installer.PackageManager, k0s installer.K0sManager, k0sctl installer.K0sctlManager) error {
	// Load install-config
	icg := installer.NewInstallConfigManager()
	if err := icg.LoadInstallConfigFromFile(c.Opts.InstallConfig); err != nil {
		return fmt.Errorf("failed to load install-config: %w", err)
	}

	config := icg.GetInstallConfig()

	if !config.Kubernetes.ManagedByCodesphere {
		return fmt.Errorf("install-config specifies external Kubernetes, k0s installation is only supported for Codesphere-managed Kubernetes")
	}

	// Determine k0s version
	k0sVersion := c.Opts.Version
	if k0sVersion == "" {
		var err error
		k0sVersion, err = k0s.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest k0s version: %w", err)
		}
		log.Printf("Using latest k0s version: %s", k0sVersion)
	}

	// Download or get k0s binary path
	var k0sBinaryPath string
	if !c.Opts.NoDownload {
		if c.Opts.Package != "" {
			// Extract the k0s binary from the package first
			if err := pm.ExtractDependency(defaultK0sPath, c.Opts.Force); err != nil {
				return fmt.Errorf("failed to extract k0s from package: %w", err)
			}
			k0sBinaryPath = pm.GetDependencyPath(defaultK0sPath)
		} else {
			var err error
			k0sBinaryPath, err = k0s.Download(k0sVersion, c.Opts.Force, false)
			if err != nil {
				return fmt.Errorf("failed to download k0s: %w", err)
			}
		}
	}

	// Download k0sctl
	log.Println("Downloading k0sctl...")
	k0sctlPath, err := k0sctl.Download(c.Opts.K0sctlVersion, c.Opts.Force, false)
	if err != nil {
		return fmt.Errorf("failed to download k0sctl: %w", err)
	}

	// Generate k0sctl configuration
	log.Println("Generating k0sctl configuration from install-config...")
	k0sctlConfig, err := installer.GenerateK0sctlConfig(config, k0sVersion, c.Opts.SSHKeyPath, k0sBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	// Write k0sctl config to file
	k0sctlConfigData, err := k0sctlConfig.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal k0sctl config: %w", err)
	}

	k0sctlConfigPath := filepath.Join(c.Env.GetOmsWorkdir(), k0sctlConfigFile)
	if err := c.FileWriter.WriteFile(k0sctlConfigPath, k0sctlConfigData, 0644); err != nil {
		return fmt.Errorf("failed to write k0sctl config: %w", err)
	}

	log.Printf("Generated k0sctl configuration at %s", k0sctlConfigPath)

	// Apply k0sctl configuration
	log.Println("Applying k0sctl configuration to deploy k0s cluster...")
	if err := k0sctl.Apply(k0sctlConfigPath, k0sctlPath, c.Opts.Force); err != nil {
		return fmt.Errorf("failed to apply k0sctl config: %w", err)
	}

	log.Println("k0s cluster deployed successfully!")
	log.Printf("To manage your cluster, use: %s kubeconfig --config %s", k0sctlPath, k0sctlConfigPath)

	return nil
}
