// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/codesphere"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// TestCmd represents the test command
type TestCmd struct {
	cmd *cobra.Command
}

// TestListCmd represents the test list command
type TestListCmd struct {
	cmd *cobra.Command
}

// AddTestCmd adds the test command and its subcommands to the root command.
func AddTestCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	test := TestCmd{
		cmd: &cobra.Command{
			Use:   "test",
			Short: "Run playlists of tests against Codesphere components",
			Long: io.Long(`Run playlists of tests against Codesphere components.

				A playlist bundles individual tests, such as a status report or a smoke test,
				into a single run with a summarized result.`),
		},
	}
	util.AddCmd(rootCmd, test.cmd)

	codesphere.AddTestCmd(test.cmd, opts)
	AddTestListCmd(test.cmd)
}

// AddTestListCmd adds the test list command to the given parent command.
func AddTestListCmd(parent *cobra.Command) {
	list := TestListCmd{
		cmd: &cobra.Command{
			Use:   "list",
			Short: "List the available tests and playlists",
			Long:  io.Long(`List the tests that can be run against a Codesphere installation and the playlists that group them.`),
			Example: util.FormatExamples("test list", []io.Example{
				{
					Cmd:  "",
					Desc: "List the available tests and playlists",
				},
			}),
		},
	}

	list.cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		codesphere.Registry(&codesphere.TestCodesphereOpts{}).Describe(cmd.OutOrStdout())
		return nil
	}

	util.AddCmd(parent, list.cmd)
}
