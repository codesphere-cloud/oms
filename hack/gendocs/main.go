// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"
	"os"

	oms "github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	// to ensure the stable binary name is used and not the temporary path
	os.Args[0] = "oms"

	err := doc.GenMarkdownTree(oms.GetRootCmd(), "docs")
	if err != nil {
		log.Fatal(err)
	}
}
