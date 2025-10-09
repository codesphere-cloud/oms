// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"log"

	v "github.com/codesphere-cloud/oms/internal/version"
	"github.com/spf13/cobra"
)

type VersionCmd struct {
	cmd *cobra.Command
}

func (c *VersionCmd) RunE(_ *cobra.Command, args []string) error {
	version := v.Build{}
	log.Printf("OMS CLI version: %s\n", version.Version())
	log.Printf("Commit: %s\n", version.Commit())
	log.Printf("Build Date: %s\n", version.BuildDate())
	log.Printf("Arch: %s\n", version.Arch())
	log.Printf("OS: %s\n", version.Os())

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
