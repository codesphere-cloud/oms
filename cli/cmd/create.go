// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// CreateCmd represents the create command group
type CreateCmd struct {
	cmd *cobra.Command
}

func AddCreateCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	create := CreateCmd{
		cmd: &cobra.Command{
			Use:   "create",
			Short: "Create resources for Codesphere",
			Long:  io.Long(`Create resources for Codesphere installations, such as test users for automated testing.`),
		},
	}
	AddCmd(rootCmd, create.cmd)

	AddCreateTestUserCmd(create.cmd, opts)
}
