// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type RevokeAPIKeyCmd struct {
	cmd  *cobra.Command
	Opts RevokeAPIKeyOpts
}

type RevokeAPIKeyOpts struct {
	*GlobalOptions
	ID string
}

func (c *RevokeAPIKeyCmd) RunE(_ *cobra.Command, args []string) error {
	p := portal.NewPortalClient()
	return c.Revoke(p)
}

func (c *RevokeAPIKeyCmd) Revoke(p portal.Portal) error {
	err := p.RevokeAPIKey(c.Opts.ID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

func AddRevokeAPIKeyCmd(list *cobra.Command, opts *GlobalOptions) {
	c := RevokeAPIKeyCmd{
		cmd: &cobra.Command{
			Use:   "api-key",
			Short: "Revoke an API key",
			Long:  io.Long(`Revoke an OMS portal API key.`),
		},
		Opts: RevokeAPIKeyOpts{GlobalOptions: opts},
	}
	c.cmd.Flags().StringVarP(&c.Opts.ID, "id", "i", "", "API key id to revoke")

	util.MarkFlagRequired(c.cmd, "id")

	c.cmd.RunE = c.RunE

	list.AddCommand(c.cmd)
}
