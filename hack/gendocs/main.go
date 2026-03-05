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
	root := oms.GetRootCmd()
	root.Use = "oms"

	root.DisableAutoGenTag = true

	identity := func(s string) string { return s }
	emptyStr := func(s string) string { return "" }
	err := doc.GenMarkdownTreeCustom(root, "docs", emptyStr, identity)
	if err != nil {
		log.Fatal(err)
	}
}
