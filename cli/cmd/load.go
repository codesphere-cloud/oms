// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// LoadCmd represents the load command.
type LoadCmd struct {
	cmd *cobra.Command
}

func AddLoadCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	load := LoadCmd{
		cmd: &cobra.Command{
			Use:   "load",
			Short: "Load resources into a local or custom registry",
			Long: io.Long(`Load resources from external sources into a local or custom registry,
				e.g. mirror images from GHCR.`),
		},
	}

	AddCmd(rootCmd, load.cmd)
	AddLoadImagesCmd(load.cmd, opts)
}
