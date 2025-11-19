// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

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
	Version string
	Package string
	Config  string
	Force   bool
}

func (c *InstallK0sCmd) RunE(_ *cobra.Command, args []string) error {
	hw := portal.NewHttpWrapper()
	env := c.Env
	pm := installer.NewPackage(env.GetOmsWorkdir(), c.Opts.Package)
	k0s := installer.NewK0s(hw, env, c.FileWriter)

	err := c.InstallK0s(pm, k0s)
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	return nil
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
			If no install config is provided, k0s will be installed with the '--single' flag.`),
			Example: formatExamplesWithBinary("install k0s", []packageio.Example{
				{Cmd: "", Desc: "Install k0s using the Go-native implementation"},
				{Cmd: "--version <version>", Desc: "Version of k0s to install"},
				{Cmd: "--package <file>", Desc: "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from"},
				{Cmd: "--k0s-config <path>", Desc: "Path to k0s configuration file, if not set k0s will be installed with the '--single' flag"},
				{Cmd: "--force", Desc: "Force new download and installation even if k0s binary exists or is already installed"},
			}, "oms-cli"),
		},
		Opts:       InstallK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", "", "Version of k0s to install")
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load k0s from")
	k0s.cmd.Flags().StringVar(&k0s.Opts.Config, "k0s-config", "", "Path to k0s configuration file")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force new download and installation")

	install.AddCommand(k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

const defaultK0sPath = "kubernetes/files/k0s"

func (c *InstallK0sCmd) InstallK0s(pm installer.PackageManager, k0s installer.K0sManager) error {
	// Default dependency path for k0s binary within package
	k0sPath := pm.GetDependencyPath(defaultK0sPath)

	var err error
	if c.Opts.Package == "" {
		k0sPath, err = k0s.Download(c.Opts.Version, c.Opts.Force, false)
		if err != nil {
			return fmt.Errorf("failed to download k0s: %w", err)
		}
	}

	err = k0s.Install(c.Opts.Config, k0sPath, c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	return nil
}
