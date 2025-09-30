package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type BetaCmd struct {
	cmd *cobra.Command
}

func AddBetaCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	beta := BetaCmd{
		cmd: &cobra.Command{
			Use:   "beta",
			Short: "Commands for early testing",
			Long: io.Long(`OMS CLI commands for early adoption and testing.
				Be aware that that usage and behavior may change as the features are developed.`),
		},
	}
	rootCmd.AddCommand(beta.cmd)

	AddExtendCmd(beta.cmd, opts)
}
