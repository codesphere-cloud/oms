// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"log"
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
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
	}
	AddVersionCmd(rootCmd)
	AddUpdateCmd(rootCmd)
	AddListCmd(rootCmd, opts)
	AddDownloadCmd(rootCmd, opts)
	AddBetaCmd(rootCmd, &opts)

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
