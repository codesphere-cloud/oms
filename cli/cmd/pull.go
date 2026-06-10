// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

// PullCmd represents the pull command
type PullCmd struct {
	cmd *cobra.Command
}

func AddPullCmd(rootCmd *cobra.Command, opts *GlobalOptions) {
	pull := PullCmd{
		cmd: &cobra.Command{
			Use:   "pull",
			Short: "Pull container images through the OMS portal registry",
			Long: io.Long(`Pull OCI container images through the OMS portal's GHCR registry proxy.
				Images are saved as OCI layers to a local directory.`),
		},
	}
	AddCmd(rootCmd, pull.cmd)

	AddPullImageCmd(pull.cmd, opts)
}
