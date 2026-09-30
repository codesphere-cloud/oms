// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/status"
	"github.com/spf13/cobra"
)

// StatusCodesphereOpts configures the status report of a Codesphere installation.
type StatusCodesphereOpts = status.Options

// StatusCodesphereCmd represents the status codesphere command.
type StatusCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *StatusCodesphereOpts
}

// RunE prints the status report and fails the command if the installation is not ready.
func (c *StatusCodesphereCmd) RunE(cmd *cobra.Command, _ []string) error {
	client, err := codesphere.NewClient(c.Opts.BaseURL, c.Opts.Token)
	if err != nil {
		return fmt.Errorf("failed to create Codesphere client: %w", err)
	}

	c.Opts.Client = client

	report := status.Fetch(cmd.Context(), c.Opts)
	status.Print(cmd.OutOrStdout(), c.Opts.BaseURL, report)

	if !report.Ready {
		return fmt.Errorf("codesphere installation is not ready")
	}

	return nil
}

// AddStatusCmd adds the status codesphere command to the given parent command.
func AddStatusCmd(parent *cobra.Command, _ *util.GlobalOptions) {
	c := StatusCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Check the status of a Codesphere installation",
			Long: csio.Long(`Check whether a Codesphere installation is reachable and ready to use,
				by querying the Codesphere API.`),
			Example: util.FormatExamples("status codesphere", []csio.Example{
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN",
					Desc: "Check the status of a Codesphere installation",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --wait",
					Desc: "Block and retry until the Codesphere installation is ready",
				},
			}),
		},
		Opts: &StatusCodesphereOpts{},
	}
	c.cmd.Flags().StringVar(&c.Opts.BaseURL, "baseurl", "", "Base URL of the Codesphere API")
	c.cmd.Flags().StringVar(&c.Opts.Token, "token", "", "API token for authentication")
	c.cmd.Flags().BoolVar(&c.Opts.Wait, "wait", false, "Block and retry until the installation is ready")
	c.cmd.Flags().DurationVar(&c.Opts.Timeout, "timeout", status.DefaultTimeout, "Timeout when waiting for the installation to become ready")

	util.MarkFlagRequired(c.cmd, "baseurl")
	util.MarkFlagRequired(c.cmd, "token")

	c.cmd.RunE = c.RunE

	util.AddCmd(parent, c.cmd)
}
