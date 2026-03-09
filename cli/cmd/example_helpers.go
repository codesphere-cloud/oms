// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/version"
)

// formatExamples builds an Example string similar to io.FormatExampleCommands
// it prefixes commands with a stable binary name (e.g. "oms") instead of temporary go-build paths
func formatExamples(cmdName string, examples []io.Example) string {
	var b strings.Builder
	for i, ex := range examples {
		if ex.Desc != "" {
			b.WriteString("# ")
			b.WriteString(ex.Desc)
			b.WriteString("\n")
		}
		b.WriteString("$ ")
		build := version.Build{}
		b.WriteString(build.BinName())
		b.WriteString(" ")
		b.WriteString(cmdName)
		if ex.Cmd != "" {
			b.WriteString(" ")
			b.WriteString(ex.Cmd)
		}
		b.WriteString("\n")
		if i < len(examples)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
