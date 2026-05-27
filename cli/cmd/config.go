// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type ConfigCmd struct {
	cmd *cobra.Command
}

func AddConfigCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	config := ConfigCmd{
		cmd: &cobra.Command{
			Use:   "config",
			Short: "Work with OMS configuration files",
			Long:  io.Long(`Work with OMS configuration files.`),
		},
	}

	AddConfigTemplateCmd(config.cmd, opts)
	AddCmd(rootCmd, config.cmd)
}
