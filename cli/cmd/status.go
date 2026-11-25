// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/codesphere"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// StatusCmd represents the status command
type StatusCmd struct {
	cmd *cobra.Command
}

// AddStatusCmd adds the status command and its subcommands to the root command.
func AddStatusCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	status := StatusCmd{
		cmd: &cobra.Command{
			Use:   "status",
			Short: "Check the status of Codesphere components",
			Long:  io.Long(`Check whether Codesphere installations or components are up and ready.`),
		},
	}
	util.AddCmd(rootCmd, status.cmd)

	codesphere.AddStatusCmd(status.cmd, opts)
}
