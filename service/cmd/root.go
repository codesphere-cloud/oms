// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"os"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

func Execute() {
	rootCmd := &cobra.Command{
		Use:   "service",
		Short: "Codesphere Operations Management System",
		Long: io.Long(`This is the OMS standalone service, which can be used to manage and observe Codesphere installations.

			This area is work in progress! OMS is under heavy development so please take a look back soon!`),
	}

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
