// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type UpdateCmd struct {
	cmd *cobra.Command
}

func (c *UpdateCmd) RunE(_ *cobra.Command, args []string) error {
	fmt.Printf("running %s", c.cmd.Use)

	return nil
}

func AddUpdateCmd(rootCmd *cobra.Command, opts GlobalOptions) {
	updateCmd := UpdateCmd{
		cmd: &cobra.Command{
			Use:   "update",
			Short: "Update OMS related resources",
			Long:  `Updates resources, e.g. OMS or OMS API keys.`,
		},
	}

	updateCmd.cmd.RunE = updateCmd.RunE

	AddDownloadPackageCmd(updateCmd.cmd, opts)
	addOmsUpdateCmd(updateCmd.cmd)
	addApiKeyUpdateCmd(updateCmd.cmd)

	rootCmd.AddCommand(updateCmd.cmd)
}
