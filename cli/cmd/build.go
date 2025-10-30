// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// BuildCmd represents the build command
type BuildCmd struct {
	cmd *cobra.Command
}

func AddBuildCmd(rootCmd *cobra.Command, opts GlobalOptions) {
	build := BuildCmd{
		cmd: &cobra.Command{
			Use:   "build",
			Short: "Build and push images to a registry",
			Long:  io.Long(`Build and push container images to a registry using the provided configuration.`),
		},
	}
	AddBuildImagesCmd(build.cmd, opts)
	AddBuildImageCmd(build.cmd, opts)

	rootCmd.AddCommand(build.cmd)
}
