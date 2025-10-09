// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// DownloadCmd represents the download command
type DownloadCmd struct {
	cmd *cobra.Command
}

func AddDownloadCmd(rootCmd *cobra.Command, opts GlobalOptions) {
	download := DownloadCmd{
		cmd: &cobra.Command{
			Use:   "download",
			Short: "Download resources available through OMS",
			Long: io.Long(`Download resources managed by or available for OMS,
				e.g. available Codesphere packages`),
		},
	}
	rootCmd.AddCommand(download.cmd)

	AddDownloadPackageCmd(download.cmd, opts)
}
