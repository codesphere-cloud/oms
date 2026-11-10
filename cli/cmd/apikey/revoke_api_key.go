// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package apikey

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/spf13/cobra"
)

type RevokeAPIKeyCmd struct {
	cmd  *cobra.Command
	Opts RevokeAPIKeyOpts
}

type RevokeAPIKeyOpts struct {
	*util.GlobalOptions
	ID string
}

// RunE revokes the API key with the real portal client.
func (c *RevokeAPIKeyCmd) RunE(cmd *cobra.Command, _ []string) error {
	p := portal.NewPortalClient()
	return c.Revoke(cmd.Context(), p)
}

// Revoke revokes the API key given in the options.
func (c *RevokeAPIKeyCmd) Revoke(ctx context.Context, p portal.Portal) error {
	err := p.RevokeAPIKey(ctx, c.Opts.ID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

func AddRevokeCmd(list *cobra.Command, opts *util.GlobalOptions) {
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

	util.AddCmd(list, c.cmd)
}
