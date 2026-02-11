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
	"github.com/codesphere-cloud/oms/internal/installer/files"
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
	config, err := c.loadInstallConfig()
	if err != nil {
		return err
	}

	k0sVersion, err := c.determineK0sVersion(k0s)
	if err != nil {
		return err
	}

	k0sBinaryPath, err := c.getK0sBinaryPath(pm, k0s, k0sVersion)
	if err != nil {
		return err
	}

	k0sctlPath, err := c.downloadK0sctl(k0sctl)
	if err != nil {
		return err
	}

	k0sctlConfigPath, err := c.generateK0sctlConfig(config, k0sVersion, k0sBinaryPath)
	if err != nil {
		return err
	}

	return c.deployK0sCluster(k0sctl, k0sctlPath, k0sctlConfigPath)
}

func (c *InstallK0sCmd) loadInstallConfig() (*files.RootConfig, error) {
	icg := installer.NewInstallConfigManager()
	if err := icg.LoadInstallConfigFromFile(c.Opts.InstallConfig); err != nil {
		return nil, fmt.Errorf("failed to load install-config: %w", err)
	}

	config := icg.GetInstallConfig()

	if !config.Kubernetes.ManagedByCodesphere {
		return nil, fmt.Errorf("install-config specifies external Kubernetes, k0s installation is only supported for Codesphere-managed Kubernetes")
	}

	return config, nil
}

func (c *InstallK0sCmd) determineK0sVersion(k0s installer.K0sManager) (string, error) {
	k0sVersion := c.Opts.Version
	if k0sVersion == "" {
		var err error
		k0sVersion, err = k0s.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest k0s version: %w", err)
		}
		log.Printf("Using latest k0s version: %s", k0sVersion)
	}
	return k0sVersion, nil
}

func (c *InstallK0sCmd) getK0sBinaryPath(pm installer.PackageManager, k0s installer.K0sManager, k0sVersion string) (string, error) {
	if c.Opts.NoDownload {
		return "", nil
	}

	if c.Opts.Package != "" {
		if err := pm.ExtractDependency(defaultK0sPath, c.Opts.Force); err != nil {
			return "", fmt.Errorf("failed to extract k0s from package: %w", err)
		}
		return pm.GetDependencyPath(defaultK0sPath), nil
	}

	k0sBinaryPath, err := k0s.Download(k0sVersion, c.Opts.Force, false)
	if err != nil {
		return "", fmt.Errorf("failed to download k0s: %w", err)
	}
	return k0sBinaryPath, nil
}

func (c *InstallK0sCmd) downloadK0sctl(k0sctl installer.K0sctlManager) (string, error) {
	log.Println("Downloading k0sctl...")
	k0sctlPath, err := k0sctl.Download(c.Opts.K0sctlVersion, c.Opts.Force, false)
	if err != nil {
		return "", fmt.Errorf("failed to download k0sctl: %w", err)
	}
	return k0sctlPath, nil
}

func (c *InstallK0sCmd) generateK0sctlConfig(config *files.RootConfig, k0sVersion string, k0sBinaryPath string) (string, error) {
	log.Println("Generating k0sctl configuration from install-config...")
	k0sctlConfig, err := installer.GenerateK0sctlConfig(config, k0sVersion, c.Opts.SSHKeyPath, k0sBinaryPath)
	if err != nil {
		return "", fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	k0sctlConfigData, err := k0sctlConfig.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal k0sctl config: %w", err)
	}

	k0sctlConfigPath := filepath.Join(c.Env.GetOmsWorkdir(), k0sctlConfigFile)
	if err := c.FileWriter.WriteFile(k0sctlConfigPath, k0sctlConfigData, 0644); err != nil {
		return "", fmt.Errorf("failed to write k0sctl config: %w", err)
	}

	log.Printf("Generated k0sctl configuration at %s", k0sctlConfigPath)
	return k0sctlConfigPath, nil
}

func (c *InstallK0sCmd) deployK0sCluster(k0sctl installer.K0sctlManager, k0sctlPath string, k0sctlConfigPath string) error {
	log.Println("Applying k0sctl configuration to deploy k0s cluster...")
	if err := k0sctl.Apply(k0sctlConfigPath, k0sctlPath, c.Opts.Force); err != nil {
		return fmt.Errorf("failed to apply k0sctl config: %w", err)
	}

	log.Println("k0s cluster deployed successfully!")
	log.Printf("To manage your cluster, use: %s kubeconfig --config %s", k0sctlPath, k0sctlConfigPath)

	return nil
}
