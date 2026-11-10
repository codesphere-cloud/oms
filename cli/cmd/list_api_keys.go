// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

// ListAPIKeysCmd implements the list api-keys command.
type ListAPIKeysCmd struct {
	cmd         *cobra.Command
	TableWriter intutil.TableWriter
}

// RunE lists the API keys from the portal.
func (c *ListAPIKeysCmd) RunE(_ *cobra.Command, _ []string) error {
	p := portal.NewPortalClient()

	keys, err := p.ListAPIKeys()
	if err != nil {
		return fmt.Errorf("failed to list api keys: %w", err)
	}

	c.PrintKeysTable(keys)

	return nil
}

// AddListAPIKeysCmd registers the api-keys command on the list command.
func AddListAPIKeysCmd(list *cobra.Command, _ *util.GlobalOptions) {
	c := ListAPIKeysCmd{
		cmd: &cobra.Command{
			Use:   "api-keys",
			Short: "List API keys",
			Long:  io.Long(`List API keys registered in the OMS portal.`),
		},
		TableWriter: intutil.GetTableWriter(),
	}

	c.cmd.RunE = c.RunE

	util.AddCmd(list, c.cmd)
}

// PrintKeysTable renders the given API keys as a table.
func (c *ListAPIKeysCmd) PrintKeysTable(keys []portal.ApiKey) {
	c.TableWriter.AppendHeader(table.Row{"ID", "Owner", "Organization", "Role", "Created", "Expires"})

	for _, k := range keys {
		c.TableWriter.AppendRow(table.Row{k.KeyID, k.Owner, k.Organization, k.Role, k.CreatedAt, k.ExpiresAt})
	}

	c.TableWriter.Render()
}
