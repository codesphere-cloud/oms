/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type List struct {
	cmd *cobra.Command
}

func (i *List) Run(_ *cobra.Command, args []string) {
	fmt.Println("list called")
}

func addListCmd(rootCmd *cobra.Command) {
	list := List{
		cmd: &cobra.Command{
			Use:   "list",
			Short: "A brief description of your command",
			Long: `A longer description that spans multiple lines and likely contains examples
			and usage of using your command. For example:

			Cobra is a CLI library for Go that empowers applications.
			This application is a tool to generate the needed files
			to quickly create a Cobra application.`,
		},
	}
	list.cmd.Run = list.Run

	addListCodesphereCmd(list.cmd)
	rootCmd.AddCommand(list.cmd)
}
