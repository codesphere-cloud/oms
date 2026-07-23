// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// DownloadCmd represents the download command
type DownloadCmd struct {
	cmd *cobra.Command
}

func AddDownloadCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	download := DownloadCmd{
		cmd: &cobra.Command{
			Use:   "download",
			Short: "Download resources available through OMS",
			Long: io.Long(`Download resources managed by or available for OMS,
				e.g. available Codesphere packages`),
		},
	}
	util.AddCmd(rootCmd, download.cmd)

	AddDownloadPackageCmd(download.cmd, opts)
	AddDownloadK0sCmd(download.cmd, opts)
}
