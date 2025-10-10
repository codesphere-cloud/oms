// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type UpdateAPIKeyCmd struct {
	Opts UpdateAPIKeyOpts
}

type UpdateAPIKeyOpts struct {
	GlobalOptions
	APIKeyID     string
	ExpiresAtStr string
}

func AddApiKeyUpdateCmd(parentCmd *cobra.Command) {
	cmdState := &UpdateAPIKeyCmd{
		Opts: UpdateAPIKeyOpts{},
	}

	apiKeyCmd := &cobra.Command{
		Use:   "api-key",
		Short: "Update an API key's expiration date",
		Long:  `Updates the expiration date for a given API key using the --id and --valid-to flags.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := portal.NewPortalClient()
			return cmdState.UpdateAPIKey(p)
		},
	}

	apiKeyCmd.Flags().StringVarP(&cmdState.Opts.APIKeyID, "id", "i", "", "The ID of the API key to update")
	apiKeyCmd.Flags().StringVarP(&cmdState.Opts.ExpiresAtStr, "valid-to", "v", "", "The new expiration date in RFC3339 format (e.g., \"2025-12-31T23:59:59Z\")")

	util.MarkFlagRequired(apiKeyCmd, "id")
	util.MarkFlagRequired(apiKeyCmd, "valid-to")

	parentCmd.AddCommand(apiKeyCmd)
}

func (c *UpdateAPIKeyCmd) UpdateAPIKey(p portal.Portal) error {
	expiresAt, err := time.Parse(time.RFC3339, c.Opts.ExpiresAtStr)

	if err != nil {
		return fmt.Errorf("invalid date format for <valid-to>: %w", err)
	}

	if err := p.UpdateAPIKey(c.Opts.APIKeyID, expiresAt); err != nil {
		return fmt.Errorf("failed to update API key: %w", err)
	}

	fmt.Printf("Successfully updated API key '%s' with new expiration date %s.\n", c.Opts.APIKeyID, expiresAt.Format(time.RFC1123))
	return nil
}
