/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/version"
	"github.com/spf13/cobra"
)

type VersionCmd struct {
	cmd *cobra.Command
}

func (c *VersionCmd) RunE(_ *cobra.Command, args []string) error {
	fmt.Printf("OMS CLI version: %s\n", version.Version())
	fmt.Printf("Commit: %s\n", version.Commit())
	fmt.Printf("Build Date: %s\n", version.BuildDate())
	fmt.Printf("Arch: %s\n", version.Arch())
	fmt.Printf("OS: %s\n", version.Os())

	return nil
}

func AddVersionCmd(rootCmd *cobra.Command) {
	version := VersionCmd{
		cmd: &cobra.Command{
			Use:   "version",
			Short: "Print version",
			Long:  `Print current version of OMS.`,
		},
	}
	rootCmd.AddCommand(version.cmd)
	version.cmd.RunE = version.RunE
}
