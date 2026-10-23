// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// CleanupCmd represents the cleanup command
type CleanupCmd struct {
	cmd *cobra.Command
}

// AddCleanupCmd adds the cleanup command and its subcommands to the root command
func AddCleanupCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	cleanup := CleanupCmd{
		cmd: &cobra.Command{
			Use:   "cleanup",
			Short: "Remove installed components",
			Long:  io.Long(`Remove components installed by OMS, e.g. to recover from a failed installation.`),
		},
	}
	util.AddCmd(rootCmd, cleanup.cmd)

	k0s.AddCleanupCmd(cleanup.cmd, opts)
}
