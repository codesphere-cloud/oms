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
	Config string
	Force  bool
}

func (c *InstallK0sCmd) RunE(_ *cobra.Command, args []string) error {
	hw := portal.NewHttpWrapper()
	env := c.Env
	k0s := installer.NewK0s(hw, env, c.FileWriter)

	if !k0s.BinaryExists() || c.Opts.Force {
		err := k0s.Download(c.Opts.Force, false)
		if err != nil {
			return fmt.Errorf("failed to download k0s: %w", err)
		}
	}

	err := k0s.Install(c.Opts.Config, c.Opts.Force)
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
			Long: packageio.Long(`Install k0s, a zero friction Kubernetes distribution, 
				using a Go-native implementation. This will download the k0s 
				binary directly to the OMS workdir, if not already present, and install it.`),
			Example: formatExamplesWithBinary("install k0s", []packageio.Example{
				{Cmd: "", Desc: "Install k0s using the Go-native implementation"},
				{Cmd: "--config <path>", Desc: "Path to k0s configuration file, if not set k0s will be installed with the '--single' flag"},
				{Cmd: "--force", Desc: "Force new download and installation even if k0s binary exists or is already installed"},
			}, "oms-cli"),
		},
		Opts:       InstallK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Config, "config", "c", "", "Path to k0s configuration file")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force new download and installation")

	install.AddCommand(k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}
