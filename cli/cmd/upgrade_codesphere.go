// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// UpgradeCodesphereCmd represents the codesphere command
type UpgradeCodesphereCmd struct {
	cmd *cobra.Command
}

func AddUpgradeCodesphereCmd(upgrade *cobra.Command) {
	codesphere := UpgradeCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Coming soon: Upgrade Codesphere to the latest or a specific version",
			Long:  io.Long(`Coming soon: Upgrade Codesphere to the latest or a specific version`),
		},
	}
	upgrade.AddCommand(codesphere.cmd)
}
