// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

type TemplateCmd struct {
	cmd *cobra.Command
}

func AddTemplateCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	template := TemplateCmd{
		cmd: &cobra.Command{
			Use:   "template",
			Short: "Render OMS configuration templates",
			Long:  io.Long(`Render OMS configuration templates.`),
		},
	}

	AddTemplateConfigCmd(template.cmd, opts)
	util.AddCmd(rootCmd, template.cmd)
}
