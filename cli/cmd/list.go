package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type ListCmd struct {
	cmd *cobra.Command
}

func AddListCmd(rootCmd *cobra.Command, opts GlobalOptions) {
	list := ListCmd{
		cmd: &cobra.Command{
			Use:   "list",
			Short: "List resources available through OMS",
			Long: io.Long(`List resources managed by or available for OMS,
				eg. available Codesphere packages`),
		},
	}
	rootCmd.AddCommand(list.cmd)
	AddListPackagesCmd(list.cmd, opts)
}
