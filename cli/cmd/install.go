/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// InstallCmd represents the install command
type InstallCmd struct {
	cmd *cobra.Command
}

func (c *InstallCmd) RunE(_ *cobra.Command, args []string) error {
	//Command execution goes here

	fmt.Printf("running %s", c.cmd.Use)

	return nil
}

func AddInstallCmd(rootCmd *cobra.Command) {
	install := InstallCmd{
		cmd: &cobra.Command{
			Use:   "install",
			Short: "A brief description of your command",
			Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		},
	}
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// install.cmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// install.cmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.AddCommand(install.cmd)
	install.cmd.RunE = install.RunE

	// Add child commands here
	AddInstallCephCmd(install.cmd)
	AddInstallCodesphereCmd(install.cmd)
}
