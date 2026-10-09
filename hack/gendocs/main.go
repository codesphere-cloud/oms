// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	oms "github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	output := flag.String("output", "docs", "directory for generated command pages")
	docusaurus := flag.Bool("docusaurus", false, "generate Docusaurus-compatible command pages")
	version := flag.String("version", "development", "OMS version used to generate the reference")

	flag.Parse()

	if err := os.MkdirAll(*output, 0755); err != nil {
		log.Fatal(err)
	}
	// Ensure the generated docs use the stable project command name.
	root := oms.GetRootCmd()
	root.Use = "oms"

	root.DisableAutoGenTag = true

	identity := func(s string) string { return s }
	emptyStr := func(s string) string { return "" }

	err := doc.GenMarkdownTreeCustom(root, *output, emptyStr, identity)
	if err != nil {
		log.Fatal(err)
	}

	if *docusaurus {
		if err := prepareDocusaurus(*output, *version); err != nil {
			log.Fatal(err)
		}
	}
}

// Keep Cobra's generated content and relative links, adding site metadata and
// removing the command heading because Docusaurus renders the page title.
func docusaurusPage(content []byte, version string) ([]byte, error) {
	heading, body, found := strings.Cut(string(content), "\n")
	if !found || !strings.HasPrefix(heading, "## ") || len(heading) <= 3 {
		return nil, fmt.Errorf("expected a Cobra command heading")
	}

	title := strings.TrimPrefix(heading, "## ")

	quotedTitle, err := json.Marshal(title)
	if err != nil {
		return nil, fmt.Errorf("encode command title %q: %w", title, err)
	}

	slug := strings.ReplaceAll(title, " ", "/")

	frontmatter := fmt.Sprintf(`---
title: %s
sidebar_label: %s
slug: /private-cloud/reference/oms-cli/%s
mdx:
  format: md
numbered_headings: false
`, quotedTitle, quotedTitle, slug)
	if title == "oms" {
		frontmatter += "sidebar_position: 1\n"
	}

	versionNote := fmt.Sprintf("Generated from OMS version `%s`.\n\n", version)

	return []byte(frontmatter + "---\n\n" + versionNote + strings.TrimLeft(body, "\n")), nil
}

func prepareDocusaurus(output, version string) error {
	pages, err := filepath.Glob(filepath.Join(output, "oms*.md"))
	if err != nil {
		return fmt.Errorf("find command pages in %s: %w", output, err)
	}

	for _, page := range pages {
		content, err := os.ReadFile(page)
		if err != nil {
			return fmt.Errorf("read command page %s: %w", page, err)
		}

		content, err = docusaurusPage(content, version)
		if err != nil {
			return fmt.Errorf("%s: %w", page, err)
		}

		if err := os.WriteFile(page, content, 0644); err != nil {
			return fmt.Errorf("write command page %s: %w", page, err)
		}
	}

	category := `{
  "label": "OMS CLI Reference",
  "position": 5,
  "link": {
    "type": "doc",
    "id": "oms"
  }
}
`
	if err := os.WriteFile(filepath.Join(output, "_category_.json"), []byte(category), 0644); err != nil {
		return fmt.Errorf("write CLI category metadata in %s: %w", output, err)
	}

	return nil
}
