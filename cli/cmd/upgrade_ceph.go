// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// UpgradeCephCmd represents the ceph command
type UpgradeCephCmd struct {
	cmd *cobra.Command
}

func AddUpgradeCephCmd(upgrade *cobra.Command) {
	ceph := UpgradeCephCmd{
		cmd: &cobra.Command{
			Use:   "ceph",
			Short: "Coming soon: Install a Ceph cluster",
			Long:  io.Long(`Coming soon: Install a Ceph cluster`),
		},
	}
	upgrade.AddCommand(ceph.cmd)
}
