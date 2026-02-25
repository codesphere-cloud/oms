// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/spf13/cobra"
)

const (
	API_KEY_ROLE_ADMIN = "Admin"
	API_KEY_ROLE_DEV   = "Dev"
	API_KEY_ROLE_EXT   = "Ext"
)

type RegisterCmd struct {
	cmd  *cobra.Command
	Opts RegisterOpts
}

type RegisterOpts struct {
	*GlobalOptions
	Owner        string
	Organization string
	Role         string
	ExpiresAt    string
}

func (c *RegisterCmd) RunE(_ *cobra.Command, args []string) error {
	p := portal.NewPortalClient()
	newKey, err := c.Register(p)
	if err != nil {
		return err
	}

	if newKey != nil {
		log.Printf("API key registered successfully!\nOwner: %s\nOrganisation: %s\nKey: %s\n", newKey.Owner, newKey.Organization, newKey.ApiKey)
	}

	return nil
}

func AddRegisterCmd(list *cobra.Command, opts *GlobalOptions) {
	c := RegisterCmd{
		cmd: &cobra.Command{
			Use:   "register",
			Short: "Register a new API key",
			Long:  io.Long(`Register a new API key for accessing the OMS portal.`),
		},
		Opts: RegisterOpts{GlobalOptions: opts},
	}
	c.cmd.Flags().StringVarP(&c.Opts.Owner, "owner", "o", "", "Owner of the new API key")
	c.cmd.Flags().StringVarP(&c.Opts.Organization, "organization", "g", "", "Organization of the new API key")
	c.cmd.Flags().StringVarP(&c.Opts.Role, "role", "r", "Ext", "Role of the new API key. Available roles: Admin, Dev, Ext")
	c.cmd.Flags().StringVarP(&c.Opts.ExpiresAt, "expires", "e", "", "Expiration date of the new API key. Default is 1 year from now. Format: RFC3339 (e.g., 2024-12-31T23:59:59Z)")

	c.cmd.RunE = c.RunE

	list.AddCommand(c.cmd)
}

func (c *RegisterCmd) Register(p portal.Portal) (*portal.ApiKey, error) {
	if c.Opts.Role != API_KEY_ROLE_ADMIN && c.Opts.Role != API_KEY_ROLE_DEV && c.Opts.Role != API_KEY_ROLE_EXT {
		return nil, fmt.Errorf("invalid role: %s. Available roles are: Admin, Dev, Ext", c.Opts.Role)
	}

	var err error
	var expiresAt time.Time
	if c.Opts.ExpiresAt != "" {
		expiresAt, err = time.Parse(time.RFC3339, c.Opts.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse expiration date: %w", err)
		}
	}

	newKey, err := p.RegisterAPIKey(c.Opts.Owner, c.Opts.Organization, c.Opts.Role, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to register API key: %w", err)
	}

	return newKey, nil
}
