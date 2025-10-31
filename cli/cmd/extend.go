// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// ExtendCmd represents the extend command
type ExtendCmd struct {
	cmd *cobra.Command
}

func AddExtendCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	extend := ExtendCmd{
		cmd: &cobra.Command{
			Use:   "extend",
			Short: "Extend Codesphere ressources such as base images.",
			Long:  io.Long(`Extend Codesphere ressources such as base images to customize them for your needs.`),
		},
	}
	rootCmd.AddCommand(extend.cmd)

	AddExtendBaseimageCmd(extend.cmd, opts)
}
