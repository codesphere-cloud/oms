// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type RevokeCmd struct {
	cmd *cobra.Command
}

func AddRevokeCmd(rootCmd *cobra.Command, opts GlobalOptions) {
	revoke := RevokeCmd{
		cmd: &cobra.Command{
			Use:   "revoke",
			Short: "Revoke resources available through OMS",
			Long: io.Long(`Revoke resources managed by or available for OMS,
				eg. api keys.`),
		},
	}
	rootCmd.AddCommand(revoke.cmd)
	AddRevokeAPIKeyCmd(revoke.cmd, opts)
}
