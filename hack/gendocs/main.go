// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	oms "github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	// to ensure the stable binary name is used and not the temporary path
	root := oms.GetRootCmd()
	if root != nil {
		root.Use = "oms-cli"
	}

	err := doc.GenMarkdownTree(root, "docs")
	if err != nil {
		log.Fatal(err)
	}
}
