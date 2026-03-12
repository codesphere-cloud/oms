// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"
)

// BetaInstallCmd represents the install command
type BetaInstallCmd struct {
	cmd *cobra.Command
}

func AddBetaInstallCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	install := BetaInstallCmd{
		cmd: &cobra.Command{
			Use:   "install",
			Short: "Install beta components",
		},
	}
	rootCmd.AddCommand(install.cmd)
	AddArgoCDCmd(install.cmd, opts)
}
