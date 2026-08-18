// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/codesphere"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// SmoketestCmd represents the smoketest command
type SmoketestCmd struct {
	cmd *cobra.Command
}

func AddSmoketestCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	smoketest := SmoketestCmd{
		cmd: &cobra.Command{
			Use:   "smoketest",
			Short: "Run smoke tests for Codesphere components",
			Long:  io.Long(`Run automated smoke tests for Codesphere installations to verify functionality.`),
		},
	}
	util.AddCmd(rootCmd, smoketest.cmd)

	codesphere.AddSmoketestCmd(smoketest.cmd, opts)
}
