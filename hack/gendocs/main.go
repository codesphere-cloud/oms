// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	oms "github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	// Ensure the generated docs use the stable project command name.
	// The Makefile expects docs/oms.md, so keep the root command Use as "oms".
	root := oms.GetRootCmd()
	if root != nil {
		root.Use = "oms"
	}

	err := doc.GenMarkdownTree(root, "docs")
	if err != nil {
		log.Fatal(err)
	}
}
