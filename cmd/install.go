/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// installCmd represents the install command

type Install struct {
	cmd *cobra.Command
}

func (i *Install) Run(_ *cobra.Command, args []string) {
	fmt.Println("install called")
}

func addInstallCmd(rootCmd *cobra.Command) {
	install := Install{
		cmd:  &cobra.Command{
			Use:   "install",
			Short: "A brief description of your command",
			Long: `A longer description that spans multiple lines and likely contains examples
			and usage of using your command. For example:

			Cobra is a CLI library for Go that empowers applications.
			This application is a tool to generate the needed files
			to quickly create a Cobra application.`,
		},
	}
	install.cmd.Run = install.Run

	rootCmd.AddCommand(install.cmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// installCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// installCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
