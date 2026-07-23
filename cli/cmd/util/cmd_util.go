// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import "github.com/spf13/cobra"

// AddCmd adds a command, inheriting the parent's Args validator if not explicitly set.
// Individual commands that need different argument rules can override this by setting their own Args validator.
func AddCmd(parent *cobra.Command, cmd *cobra.Command) {
	if cmd.Args == nil {
		cmd.Args = parent.Args
	}
	parent.AddCommand(cmd)
}

type GlobalOptions struct {
	OmsPortalApiKey string
}
