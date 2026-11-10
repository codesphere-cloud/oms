// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/apikey"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

// RevokeCmd groups the revoke subcommands.
type RevokeCmd struct {
	cmd *cobra.Command
}

// AddRevokeCmd registers the revoke command and its subcommands on the root command.
func AddRevokeCmd(rootCmd *cobra.Command, opts *util.GlobalOptions) {
	revoke := RevokeCmd{
		cmd: &cobra.Command{
			Use:   "revoke",
			Short: "Revoke resources available through OMS",
			Long: io.Long(`Revoke resources managed by or available for OMS,
				eg. api keys.`),
		},
	}
	util.AddCmd(rootCmd, revoke.cmd)
	apikey.AddRevokeCmd(revoke.cmd, opts)
}
