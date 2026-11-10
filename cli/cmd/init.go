// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// InitCmd groups the init subcommands.
type InitCmd struct {
	cmd *cobra.Command
}

// AddInitCmd registers the init command and its subcommands on the root command.
func AddInitCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	init := InitCmd{
		cmd: &cobra.Command{
			Use:   "init",
			Short: "Initialize configuration files",
			Long:  io.Long(`Initialize configuration files for Codesphere installation and other components.`),
		},
	}
	util.AddCmd(rootCmd, init.cmd)
	AddInitInstallConfigCmd(init.cmd, opts)
	k0s.AddInitK0sConfigCmd(init.cmd, opts)
}
