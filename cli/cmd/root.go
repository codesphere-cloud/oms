// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"log"
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/apikey"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// GetRootCmd adds all child commands to the root command and sets flags appropriately.
func GetRootCmd() *cobra.Command {
	opts := &util.GlobalOptions{}
	rootCmd := &cobra.Command{
		Use:   "oms",
		Short: "Codesphere Operations Management System (OMS)",
		Long: io.Long(`Codesphere Operations Management System (OMS)

			This command can be used to run common tasks related to managing codesphere installations,
			like downloading new versions.`),
		Args: cobra.NoArgs,
	}

	rootCmd.PersistentFlags().BoolVarP(&opts.Verbose, "verbose", "", false, "Enable verbose output")

	// General commands
	AddVersionCmd(rootCmd)
	AddBetaCmd(rootCmd, opts)
	AddUpdateCmd(rootCmd, opts)

	// Package commands
	AddListCmd(rootCmd, opts)
	AddDownloadCmd(rootCmd, opts)
	AddCopyCmd(rootCmd, opts)
	AddInstallCmd(rootCmd, opts)
	AddInitCmd(rootCmd, opts)
	AddTemplateCmd(rootCmd, opts)
	AddBuildCmd(rootCmd, opts)
	AddLicensesCmd(rootCmd)

	// OMS API key management commands
	apikey.AddRegisterCmd(rootCmd, opts)
	AddRevokeCmd(rootCmd, opts)

	// Smoke test commands
	AddSmoketestCmd(rootCmd, opts)

	// Resource creation commands
	AddCreateCmd(rootCmd, opts)

	// Cluster administration commands
	AddAddClusterAdminCmd(rootCmd, opts)

	silenceUsageOnRunErrors(rootCmd)

	return rootCmd
}

// silenceUsageOnRunErrors prevents Cobra from printing command usage for
// operational errors returned by RunE. Argument, flag, and required-flag
// validation happens before RunE, so those errors still include usage.
func silenceUsageOnRunErrors(command *cobra.Command) {
	if command.RunE != nil {
		runE := command.RunE
		command.RunE = func(cmd *cobra.Command, args []string) error {
			cmd.Root().SilenceUsage = true
			return runE(cmd, args)
		}
	}

	for _, child := range command.Commands() {
		silenceUsageOnRunErrors(child)
	}
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
