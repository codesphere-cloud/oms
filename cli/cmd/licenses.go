// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/tmpl"
	"github.com/spf13/cobra"
)

type LicensesCmd struct {
	cmd *cobra.Command
}

func (c *LicensesCmd) RunE(_ *cobra.Command, args []string) error {
	fmt.Println("OMS License:")
	fmt.Println(tmpl.License())

	fmt.Println("=================================")

	fmt.Println("Open source components included:")
	fmt.Println(tmpl.Notice())

	return nil
}

func AddLicensesCmd(rootCmd *cobra.Command) {
	licenses := LicensesCmd{
		cmd: &cobra.Command{
			Use:   "licenses",
			Short: "Print license information",
			Long:  `Print information about the OMS license and open source licenses of dependencies.`,
		},
	}
	rootCmd.AddCommand(licenses.cmd)
	licenses.cmd.RunE = licenses.RunE
}
