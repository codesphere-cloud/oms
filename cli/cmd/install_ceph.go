// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
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
	util.AddCmd(install, ceph.cmd)
}
