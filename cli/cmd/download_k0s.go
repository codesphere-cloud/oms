// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// DownloadK0sCmd represents the k0s download command
type DownloadK0sCmd struct {
	cmd        *cobra.Command
	Opts       DownloadK0sOpts
	Env        env.Env
	FileWriter util.FileIO
}

type DownloadK0sOpts struct {
	*GlobalOptions
	Version string
	Force   bool
	Quiet   bool
}

func (c *DownloadK0sCmd) RunE(_ *cobra.Command, args []string) error {
	hw := portal.NewHttpWrapper()
	env := c.Env
	k0s := installer.NewK0s(hw, env, c.FileWriter)

	err := c.DownloadK0s(k0s)
	if err != nil {
		return fmt.Errorf("failed to download k0s: %w", err)
	}

	return nil
}

func AddDownloadK0sCmd(download *cobra.Command, opts *GlobalOptions) {
	k0s := DownloadK0sCmd{
		cmd: &cobra.Command{
			Use:   "k0s",
			Short: "Download k0s Kubernetes distribution",
			Long: packageio.Long(`Download a k0s binary directly to the OMS workdir.
			Will download the latest version if no version is specified.`),
			Example: formatExamplesWithBinary("download k0s", []packageio.Example{
				{Cmd: "", Desc: "Download k0s using the Go-native implementation"},
				{Cmd: "--version 1.22.0", Desc: "Download a specific version of k0s"},
				{Cmd: "--quiet", Desc: "Download k0s with minimal output"},
				{Cmd: "--force", Desc: "Force download even if k0s binary exists"},
			}, "oms-cli"),
		},
		Opts:       DownloadK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", "", "Version of k0s to download")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force download even if k0s binary exists")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Quiet, "quiet", "q", false, "Suppress progress output during download")

	download.AddCommand(k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

func (c *DownloadK0sCmd) DownloadK0s(k0s installer.K0sManager) error {
	if c.Opts.Version == "" {
		version, err := k0s.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest k0s version: %w", err)
		}
		c.Opts.Version = version
	}

	k0sPath, err := k0s.Download(c.Opts.Version, c.Opts.Force, c.Opts.Quiet)
	if err != nil {
		return fmt.Errorf("failed to download k0s: %w", err)
	}

	log.Printf("k0s binary downloaded successfully at '%s'", k0sPath)

	return nil
}
