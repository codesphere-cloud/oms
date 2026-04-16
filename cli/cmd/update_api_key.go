// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type UpdateAPIKeyCmd struct {
	Opts UpdateAPIKeyOpts
}

type UpdateAPIKeyOpts struct {
	*GlobalOptions
	APIKeyID string
	ValidFor string
}

func AddApiKeyUpdateCmd(parentCmd *cobra.Command) {
	cmdState := &UpdateAPIKeyCmd{
		Opts: UpdateAPIKeyOpts{},
	}

	apiKeyCmd := &cobra.Command{
		Use:   "api-key",
		Short: "Update an API key's expiration date",
		Long:  `Updates the expiration date for a given API key using the --id and --valid-for flags.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := portal.NewPortalClient()
			return cmdState.UpdateAPIKey(p)
		},
	}

	apiKeyCmd.Flags().StringVarP(&cmdState.Opts.APIKeyID, "id", "i", "", "The ID of the API key to update")
	apiKeyCmd.Flags().StringVar(&cmdState.Opts.ValidFor, "valid-for", "", "Validity duration in days to extend the API key (e.g., 10d)")

	util.MarkFlagRequired(apiKeyCmd, "id")
	util.MarkFlagRequired(apiKeyCmd, "valid-for")

	AddCmd(parentCmd, apiKeyCmd)
}

func (c *UpdateAPIKeyCmd) UpdateAPIKey(p portal.Portal) error {
	validForDuration, err := util.GetDurationFromString(c.Opts.ValidFor)
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(validForDuration)

	if err := p.UpdateAPIKey(c.Opts.APIKeyID, expiresAt); err != nil {
		return fmt.Errorf("failed to update API key: %w", err)
	}

	log.Printf("Successfully updated API key '%s' with new expiration date %s.\n", c.Opts.APIKeyID, expiresAt.Format(time.RFC1123))
	return nil
}
