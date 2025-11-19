// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type InitCmd struct {
	cmd *cobra.Command
}

func AddInitCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	init := InitCmd{
		cmd: &cobra.Command{
			Use:   "init",
			Short: "Initialize configuration files",
			Long:  io.Long(`Initialize configuration files for Codesphere installation and other components.`),
		},
	}
	rootCmd.AddCommand(init.cmd)
	AddInitInstallConfigCmd(init.cmd, opts)
}
