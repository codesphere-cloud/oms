/*
Copyright © 2025 Codesphere Inc.
*/
package cli

import (
	"fmt"

	cs "github.com/codesphere-cloud/oms/pkg/codesphere"
	"github.com/spf13/cobra"
)

type UpgradeCodesphere struct {
	cmd        *cobra.Command
	Codesphere cs.Codesphere
}

func (u *UpgradeCodesphere) Run(_ *cobra.Command, args []string) {
	fmt.Println("upgrade codesphere called")
	fmt.Println(u.Codesphere)
}

func addUpgradeCodesphereCmd(upgradeCmd *cobra.Command) {
	upgradeCodesphere := UpgradeCodesphere{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "A brief description of your command",
			Long: `A longer description that spans multiple lines and likely contains examples
		and usage of using your command. For example:

		Cobra is a CLI library for Go that empowers applications.
		This application is a tool to generate the needed files
		to quickly create a Cobra application.`,
		},
	}
	upgradeCodesphere.cmd.Run = upgradeCodesphere.Run
	ParseCodesphereFlags(&upgradeCodesphere.Codesphere, upgradeCodesphere.cmd)
	upgradeCmd.AddCommand(upgradeCodesphere.cmd)
}
