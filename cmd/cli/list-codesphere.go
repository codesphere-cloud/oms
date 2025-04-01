/*
Copyright © 2025 Codesphere Inc.
*/
package cli

import (
	"fmt"

	cs "github.com/codesphere-cloud/oms/pkg/codesphere"
	"github.com/spf13/cobra"
)

type ListCodesphere struct {
	cmd        *cobra.Command
	Codesphere cs.Codesphere
}

func (l *ListCodesphere) Run(_ *cobra.Command, args []string) {
	fmt.Println("list codesphere called")
	fmt.Println(l.Codesphere)
}

func addListCodesphereCmd(listCmd *cobra.Command) {
	listCodesphere := ListCodesphere{
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
	listCodesphere.cmd.Run = listCodesphere.Run
	ParseCodesphereFlags(&listCodesphere.Codesphere, listCodesphere.cmd)
	listCmd.AddCommand(listCodesphere.cmd)
}
