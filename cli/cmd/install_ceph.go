// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// InstallCephCmd represents the ceph command
type InstallCephCmd struct {
	cmd *cobra.Command
}

func AddInstallCephCmd(install *cobra.Command) {
	ceph := InstallCephCmd{
		cmd: &cobra.Command{
			Use:   "ceph",
			Short: "Coming soon: Install a Ceph cluster",
			Long:  io.Long(`Coming soon: Install a Ceph cluster`),
		},
	}
	install.AddCommand(ceph.cmd)
}
