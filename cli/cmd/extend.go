package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ExtendCmd represents the extend command
type ExtendCmd struct {
	cmd *cobra.Command
}

func (c *ExtendCmd) RunE(_ *cobra.Command, args []string) error {
	//Command execution goes here

	fmt.Printf("running %s", c.cmd.Use)

	return nil
}

func AddExtendCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	extend := ExtendCmd{
		cmd: &cobra.Command{
			Use:   "extend",
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
	// extend.cmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// extend.cmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.AddCommand(extend.cmd)
	extend.cmd.RunE = extend.RunE

	// Add child commands here
	AddExtendBaseimageCmd(extend.cmd, opts)
}
