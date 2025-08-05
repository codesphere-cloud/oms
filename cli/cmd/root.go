/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type GlobalOptions struct {
	OmsPortalApiKey string
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	opts := GlobalOptions{}
	rootCmd := &cobra.Command{
		Use:   "oms",
		Short: "Codesphere Operations Management System (OMS)",
		Long: io.Long(`Codesphere Operations Management System (OMS)

			This command can be used to run common tasks related to managing codesphere installations,
			like downloading new versions.`),
	}
	AddInstallCmd(rootCmd)
	AddListCmd(rootCmd, opts)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
