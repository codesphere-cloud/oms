// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"
)

// UpgradeCmd represents the upgrade command
type UpgradeCmd struct {
	cmd *cobra.Command
}

func AddUpgradeCmd(rootCmd *cobra.Command) {
	upgrade := UpgradeCmd{
		cmd: &cobra.Command{
			Use:   "upgrade",
			Short: "Coming soon: Upgrade Codesphere OMS",
			Long:  `Coming soon: Upgrade Codesphere OMS to the latest or a specific version.`,
		},
	}

	rootCmd.AddCommand(upgrade.cmd)
}
