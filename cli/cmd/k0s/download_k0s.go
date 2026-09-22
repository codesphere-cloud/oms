// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"fmt"
	"log"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// DownloadK0sCmd represents the k0s download command
type DownloadK0sCmd struct {
	cmd        *cobra.Command
	Opts       DownloadK0sOpts
	Env        env.Env
	FileWriter intutil.FileIO
}

type DownloadK0sOpts struct {
	*util.GlobalOptions
	Version string
	Force   bool
	Airgap  bool
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

func AddDownloadCmd(download *cobra.Command, opts *util.GlobalOptions) {
	k0s := DownloadK0sCmd{
		cmd: &cobra.Command{
			Use:   "k0s",
			Short: "Download k0s Kubernetes distribution",
			Long: packageio.Long(`Download a k0s binary directly to the OMS workdir.
			Will download the latest version if no version is specified.`),
			Example: util.FormatExamples("download k0s", []packageio.Example{
				{Cmd: "", Desc: "Download k0s using the Go-native implementation"},
				{Cmd: "--version 1.22.0", Desc: "Download a specific version of k0s"},
				{Cmd: "--force", Desc: "Force download even if k0s binary exists"},
				{Cmd: "--airgapped", Desc: "Also download the airgap image bundle for that version"},
			}),
		},
		Opts:       DownloadK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: intutil.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", "", "Version of k0s to download")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force download even if k0s binary exists")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Airgap, "airgapped", "a", false, "Downloads the airgapped bundle for that version")

	util.AddCmd(download, k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

func (c *DownloadK0sCmd) DownloadK0s(k0s installer.K0sManager) error {
	version, err := resolveK0sVersion(k0s, c.Opts.Version)
	if err != nil {
		return err
	}

	k0sPath, err := k0s.Download(version, installer.DownloadOptions{
		Force:     c.Opts.Force,
		Quiet:     !c.Opts.Verbose,
		Airgapped: c.Opts.Airgap,
	})
	if err != nil {
		return fmt.Errorf("failed to download k0s: %w", err)
	}

	log.Printf("k0s binary downloaded successfully to '%s'", k0sPath)

	return nil
}
