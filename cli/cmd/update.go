// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

type UpdateCmd struct {
	cmd *cobra.Command
}

func AddUpdateCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	updateCmd := UpdateCmd{
		cmd: &cobra.Command{
			Use:   "update",
			Short: "Update OMS related resources",
			Long:  io.Long(`Updates resources, e.g. OMS or OMS API keys.`),
		},
	}

	AddOmsUpdateCmd(updateCmd.cmd)
	AddApiKeyUpdateCmd(updateCmd.cmd)
	AddUpdateDockerfileCmd(updateCmd.cmd, opts)
	AddUpdateInstallConfigCmd(updateCmd.cmd, opts)

	util.AddCmd(rootCmd, updateCmd.cmd)
}
