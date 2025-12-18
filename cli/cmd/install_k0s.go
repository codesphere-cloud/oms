// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
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
	Package       string
	InstallConfig string
	SSHKeyPath    string
	RemoteHost    string
	RemoteUser    string
	Force         bool
}

func (c *InstallK0sCmd) RunE(_ *cobra.Command, args []string) error {
	hw := portal.NewHttpWrapper()
	env := c.Env
	pm := installer.NewPackage(env.GetOmsWorkdir(), c.Opts.Package)
	k0s := installer.NewK0s(hw, env, c.FileWriter)

	return c.InstallK0s(pm, k0s)
}

func AddInstallK0sCmd(install *cobra.Command, opts *GlobalOptions) {
	k0s := InstallK0sCmd{
		cmd: &cobra.Command{
			Use:   "k0s",
			Short: "Install k0s Kubernetes distribution",
			Long: packageio.Long(`Install k0s either from the package or by downloading it.
			This will either download the k0s binary directly to the OMS workdir, if not already present, and install it
			or load the k0s binary from the provided package file and install it.
			If no version is specified, the latest version will be downloaded.
			
			You must provide a Codesphere install-config file, which will:
			- Generate a k0s configuration from the install-config
			- Optionally install k0s on remote nodes via SSH`),
			Example: formatExamplesWithBinary("install k0s", []packageio.Example{
				{Cmd: "--install-config <path>", Desc: "Path to Codesphere install-config file to generate k0s config from"},
				{Cmd: "--version <version>", Desc: "Version of k0s to install"},
				{Cmd: "--package <file>", Desc: "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from"},
				{Cmd: "--remote-host <ip>", Desc: "Remote host IP to install k0s on (requires --ssh-key-path)"},
				{Cmd: "--ssh-key-path <path>", Desc: "SSH private key path for remote installation"},
				{Cmd: "--force", Desc: "Force new download and installation even if k0s binary exists or is already installed"},
			}, "oms-cli"),
		},
		Opts:       InstallK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", "", "Version of k0s to install")
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from")
	k0s.cmd.Flags().StringVar(&k0s.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file (required)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path for remote installation")
	k0s.cmd.Flags().StringVar(&k0s.Opts.RemoteHost, "remote-host", "", "Remote host IP to install k0s on")
	k0s.cmd.Flags().StringVar(&k0s.Opts.RemoteUser, "remote-user", "root", "Remote user for SSH connection")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force new download and installation")

	_ = k0s.cmd.MarkFlagRequired("install-config")
	k0s.cmd.MarkFlagsRequiredTogether("remote-host", "ssh-key-path")

	install.AddCommand(k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

const defaultK0sPath = "kubernetes/files/k0s"

func (c *InstallK0sCmd) InstallK0s(pm installer.PackageManager, k0s installer.K0sManager) error {
	icg := installer.NewInstallConfigManager()
	if err := icg.LoadInstallConfigFromFile(c.Opts.InstallConfig); err != nil {
		return fmt.Errorf("failed to load install-config: %w", err)
	}

	config := icg.GetInstallConfig()

	if !config.Kubernetes.ManagedByCodesphere {
		return fmt.Errorf("install-config specifies external Kubernetes, k0s installation is only supported for Codesphere-managed Kubernetes")
	}

	log.Println("Generating k0s configuration from install-config...")
	k0sConfig, err := installer.GenerateK0sConfig(config)
	if err != nil {
		return fmt.Errorf("failed to generate k0s config: %w", err)
	}

	k0sConfigData, err := k0sConfig.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal k0s config: %w", err)
	}

	// allow temp directory in tests
	k0sConfigPath := "/etc/k0s/k0s.yaml"
	usedTempPath := false

	if err := os.MkdirAll(filepath.Dir(k0sConfigPath), 0755); err != nil {
		tmpK0sConfigPath := filepath.Join(os.TempDir(), "k0s-config.yaml")
		if err := os.WriteFile(tmpK0sConfigPath, k0sConfigData, 0644); err != nil {
			return fmt.Errorf("failed to write k0s config: %w", err)
		}
		k0sConfigPath = tmpK0sConfigPath
		usedTempPath = true
		log.Printf("Generated k0s configuration at %s (using temp path due to permissions)", k0sConfigPath)
	} else {
		if err := os.WriteFile(k0sConfigPath, k0sConfigData, 0644); err != nil {
			return fmt.Errorf("failed to write k0s config: %w", err)
		}
		log.Printf("Generated k0s configuration at %s", k0sConfigPath)
	}

	// Clean up temp file if used
	if usedTempPath {
		defer func() { _ = os.Remove(k0sConfigPath) }()
	}

	k0sPath := pm.GetDependencyPath(defaultK0sPath)
	if c.Opts.Package == "" {
		k0sPath, err = k0s.Download(c.Opts.Version, c.Opts.Force, false)
		if err != nil {
			return fmt.Errorf("failed to download k0s: %w", err)
		}
	}

	if c.Opts.RemoteHost != "" {
		return c.InstallK0sRemote(config, k0sPath, k0sConfigPath)
	}

	err = k0s.Install(k0sConfigPath, k0sPath, c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	log.Println("k0s installed successfully using configuration from install-config")
	return nil
}

func (c *InstallK0sCmd) InstallK0sRemote(config *files.RootConfig, k0sBinaryPath string, k0sConfigPath string) error {
	log.Printf("Installing k0s on remote host %s", c.Opts.RemoteHost)

	nm := &node.NodeManager{
		FileIO:  c.FileWriter,
		KeyPath: c.Opts.SSHKeyPath,
	}

	remoteNode := &node.Node{
		ExternalIP: c.Opts.RemoteHost,
		InternalIP: c.Opts.RemoteHost,
		Name:       "k0s-node",
		User:       c.Opts.RemoteUser,
	}

	if err := remoteNode.InstallK0s(nm, k0sBinaryPath, k0sConfigPath, c.Opts.Force); err != nil {
		return fmt.Errorf("failed to install k0s on remote host: %w", err)
	}

	log.Printf("k0s successfully installed on remote host %s", c.Opts.RemoteHost)
	return nil
}
