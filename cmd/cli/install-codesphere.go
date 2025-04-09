/*
Copyright © 2025 Codesphere Inc.
*/
package cli

import (
	"fmt"

	cs "github.com/codesphere-cloud/oms/pkg/codesphere"
	"github.com/spf13/cobra"
)

type InstallCodesphere struct {
	cmd        *cobra.Command
	Codesphere cs.Codesphere
}

func (i *InstallCodesphere) Run(_ *cobra.Command, args []string) {
	fmt.Println("install codesphere called")
	fmt.Println(i.Codesphere)
}

func addInstallCodesphereCmd(installCmd *cobra.Command) {
	installCodesphere := InstallCodesphere{
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
	installCodesphere.cmd.Run = installCodesphere.Run
	ParseCodesphereFlags(&installCodesphere.Codesphere, installCodesphere.cmd)

	installCmd.AddCommand(installCodesphere.cmd)
}
