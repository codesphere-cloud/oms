// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/spf13/cobra"
)

type GlobalOptions struct {
	OmsPortalApiKey string
}

// GetRootCmd adds all child commands to the root command and sets flags appropriately.
func GetRootCmd() *cobra.Command {
	opts := GlobalOptions{}
	rootCmd := &cobra.Command{
		Use:   "oms",
		Short: "Codesphere Operations Management System (OMS)",
		Long: io.Long(`Codesphere Operations Management System (OMS)

			This command can be used to run common tasks related to managing codesphere installations,
			like downloading new versions.`),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			apiKey := os.Getenv("OMS_PORTAL_API_KEY")

			if len(apiKey) == 22 {
				fmt.Fprintf(os.Stderr, "Warning: You used an old API key format.\n")
				fmt.Fprintf(os.Stderr, "Attempting to upgrade to the new format...\n\n")

				portalClient := portal.NewPortalClient()
				newApiKey, err := portalClient.GetApiKeyByHeader(apiKey)

				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: Failed to upgrade old API key: %v\n", err)
					return
				}

				if err := os.Setenv("OMS_PORTAL_API_KEY", newApiKey); err != nil {
					fmt.Fprintf(os.Stderr, "Error: Failed to set environment variable: %v\n", err)
					return
				}
				opts.OmsPortalApiKey = newApiKey

				fmt.Fprintf(os.Stderr, "Please update your environment variable:\n\n")
				fmt.Fprintf(os.Stderr, "  export OMS_PORTAL_API_KEY='%s'\n\n", newApiKey)
			}
		},
	}
	// General commands
	AddVersionCmd(rootCmd)
	AddBetaCmd(rootCmd, &opts)
	AddUpdateCmd(rootCmd, opts)

	// Package commands
	AddListCmd(rootCmd, opts)
	AddDownloadCmd(rootCmd, opts)
	AddInstallCmd(rootCmd, &opts)
	AddLicensesCmd(rootCmd)

	// OMS API key management commands
	AddRegisterCmd(rootCmd, opts)
	AddRevokeCmd(rootCmd, opts)

	return rootCmd
}

// Execute executes the root command. This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	//Disable printing timestamps on log lines
	log.SetFlags(0)

	err := GetRootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}
