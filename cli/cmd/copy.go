// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// CopyCmd represents the copy command.
type CopyCmd struct {
	cmd *cobra.Command
}

// AddCopyCmd adds the copy command group to the root command.
func AddCopyCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	copyCmd := CopyCmd{cmd: &cobra.Command{
		Use:   "copy",
		Short: "Copy resources between locations",
		Long: csio.Long(`Copy resources managed by OMS between locations,
			e.g. package container images and OCI Helm charts between registries.`),
	}}
	util.AddCmd(rootCmd, copyCmd.cmd)
	AddCopyPackageCmd(copyCmd.cmd, opts)
}
