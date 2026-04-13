// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type UpdateCmd struct {
	cmd *cobra.Command
}

func AddUpdateCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
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

	AddCmd(rootCmd, updateCmd.cmd)
}
