// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Command gennotice generates NOTICE while retaining known metadata when URL or
// license discovery temporarily returns Unknown for an unchanged dependency.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type dependency struct {
	module  string
	version string
}

type noticeEntry struct {
	dependency
	license    string
	licenseURL string
}

const noticeSeparator = "\n----------\n"

func main() {
	if err := generateNotice(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateNotice() error {
	previous, err := os.ReadFile("NOTICE")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing NOTICE: %w", err)
	}

	command := exec.Command("go", "tool", "go-licenses", "report", "--template", ".NOTICE.template", "./...")
	command.Stderr = os.Stderr

	generated, err := command.Output()
	if err != nil {
		return fmt.Errorf("generate license report: %w", err)
	}

	if err := os.WriteFile("NOTICE", []byte(preserveKnownLicenses(string(previous), string(generated))), 0644); err != nil {
		return fmt.Errorf("write NOTICE: %w", err)
	}

	return nil
}

// preserveKnownLicenses keeps known metadata when discovery returns Unknown for
// the same module and version. New versions must use their own license results.
func preserveKnownLicenses(previous, generated string) string {
	known := map[dependency]noticeEntry{}

	for _, block := range strings.Split(previous, noticeSeparator) {
		entry := parseNoticeEntry(block)
		if entry.module != "" && entry.version != "" && entry.version != "Unknown" {
			known[entry.dependency] = entry
		}
	}

	blocks := strings.Split(generated, noticeSeparator)
	for i, block := range blocks {
		entry := parseNoticeEntry(block)

		old, exists := known[entry.dependency]
		if !exists {
			continue
		}

		lines := strings.Split(block, "\n")
		for j, line := range lines {
			if line == "License: Unknown" && old.license != "" && old.license != "Unknown" {
				lines[j] = "License: " + old.license
			}

			if line == "License URL: Unknown" && old.licenseURL != "" && old.licenseURL != "Unknown" {
				lines[j] = "License URL: " + old.licenseURL
			}
		}

		blocks[i] = strings.Join(lines, "\n")
	}

	return strings.Join(blocks, noticeSeparator)
}

func parseNoticeEntry(block string) noticeEntry {
	entry := noticeEntry{}

	for _, line := range strings.Split(block, "\n") {
		field, value, found := strings.Cut(line, ": ")
		if !found {
			continue
		}

		switch field {
		case "Module":
			entry.module = value
		case "Version":
			entry.version = value
		case "License":
			entry.license = value
		case "License URL":
			entry.licenseURL = value
		}
	}

	return entry
}
