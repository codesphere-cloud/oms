// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"

	"github.com/spf13/cobra"
)

func MarkFlagRequired(cmd *cobra.Command, name string) {
	err := cmd.MarkFlagRequired(name)
	if err != nil {
		panic(fmt.Errorf("failed to mark flag as required, please check existence: %w", err))
	}
}
