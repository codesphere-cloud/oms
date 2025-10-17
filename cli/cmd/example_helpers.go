package cmd

import (
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
)

// formatExamplesWithBinary builds an Example string similar to io.FormatExampleCommands
// but prefixes commands with a stable binary name (e.g. "oms-cli") instead of temporary go-build paths
func formatExamplesWithBinary(cmdName string, examples []io.Example, binaryName string) string {
	var b strings.Builder
	for i, ex := range examples {
		if ex.Desc != "" {
			b.WriteString("# ")
			b.WriteString(ex.Desc)
			b.WriteString("\n")
		}
		b.WriteString("$ ")
		b.WriteString(binaryName)
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
