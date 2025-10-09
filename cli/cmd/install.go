// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// InstallCmd represents the install command
type InstallCmd struct {
	cmd *cobra.Command
}

func AddInstallCmd(rootCmd *cobra.Command) {
	install := InstallCmd{
		cmd: &cobra.Command{
			Use:   "install",
			Short: "Coming soon: Install Codesphere and other components",
			Long:  io.Long(`Coming soon: Install Codesphere and other components like Ceph and PostgreSQL.`),
		},
	}
	rootCmd.AddCommand(install.cmd)
}
