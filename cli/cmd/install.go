// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/codesphere"
	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// InstallCmd represents the install command
type InstallCmd struct {
	cmd *cobra.Command
}

func AddInstallCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	install := InstallCmd{
		cmd: &cobra.Command{
			Use:   "install",
			Short: "Install Codesphere and other components",
			Long:  io.Long(`Install Codesphere and other components like Ceph and PostgreSQL.`),
		},
	}
	util.AddCmd(rootCmd, install.cmd)

	codesphere.AddInstallCmd(install.cmd, opts)
	k0s.AddInstallCmd(install.cmd, opts)
	AddInstallOpenBaoCmd(install.cmd, opts)
}
