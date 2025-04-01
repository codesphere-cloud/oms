/*
Copyright © 2025 Codesphere Inc.
*/
package cli

import (
	"os"

	cs "github.com/codesphere-cloud/oms/pkg/codesphere"
	"github.com/spf13/cobra"
)

func Execute() {
	var rootCmd = &cobra.Command{
		Use:   "oms",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
	examples and usage of using your application. For example:

	Cobra is a CLI library for Go that empowers applications.
	This application is a tool to generate the needed files
	to quickly create a Cobra application.`,
	}
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	addInstallCmd(rootCmd)
	addUpgradeCmd(rootCmd)
	addListCmd(rootCmd)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func ParseCodesphereFlags(codesphere *cs.Codesphere, cmd *cobra.Command) {
	codesphere.Name = cmd.Flags().StringP("name", "n", "MyCodesphere", "Name of Codesphere instance")
}
