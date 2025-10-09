// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// InstallCodesphereCmd represents the codesphere command
type InstallCodesphereCmd struct {
	cmd *cobra.Command
}

func AddInstallCodesphereCmd(install *cobra.Command) {
	codesphere := InstallCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Coming soon: Install a Codesphere instance",
			Long:  io.Long(`Coming soon: Install a Codesphere instance`),
		},
	}
	install.AddCommand(codesphere.cmd)
}
