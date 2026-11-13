// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"log"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	v "github.com/codesphere-cloud/oms/internal/version"
	"github.com/spf13/cobra"
)

// VersionCmd implements the version command.
type VersionCmd struct {
	cmd *cobra.Command
}

// RunE prints the version information of the OMS CLI.
func (c *VersionCmd) RunE(_ *cobra.Command, _ []string) error {
	version := v.Build{}
	log.Printf("OMS CLI version: %s\n", version.Version())
	log.Printf("Commit: %s\n", version.Commit())
	log.Printf("Build Date: %s\n", version.BuildDate())
	log.Printf("Arch: %s\n", version.Arch())
	log.Printf("OS: %s\n", version.Os())

	return nil
}

// AddVersionCmd registers the version command on the root command.
func AddVersionCmd(rootCmd *cobra.Command) {
	version := VersionCmd{
		cmd: &cobra.Command{
			Use:   "version",
			Short: "Print version",
			Long:  `Print current version of OMS.`,
		},
	}
	util.AddCmd(rootCmd, version.cmd)
	version.cmd.RunE = version.RunE
}
