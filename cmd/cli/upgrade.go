/*
Copyright © 2025 Codesphere Inc.
*/
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type Upgrade struct {
	cmd *cobra.Command
}

func (i *Upgrade) Run(_ *cobra.Command, args []string) {
	fmt.Println("upgrade called")
}

func addUpgradeCmd(rootCmd *cobra.Command) {
	upgrade := Upgrade{
		cmd: &cobra.Command{
			Use:   "upgrade",
			Short: "A brief description of your command",
			Long: `A longer description that spans multiple lines and likely contains examples
			and usage of using your command. For example:

			Cobra is a CLI library for Go that empowers applications.
			This application is a tool to generate the needed files
			to quickly create a Cobra application.`,
		},
	}
	upgrade.cmd.Run = upgrade.Run

	addUpgradeCodesphereCmd(upgrade.cmd)
	rootCmd.AddCommand(upgrade.cmd)
}
