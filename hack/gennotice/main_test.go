// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestPreserveKnownLicenses(t *testing.T) {
	const previous = "Header\n\n----------\nModule: example.com/library\nVersion: v1.0.0\nLicense: MIT\nLicense URL: https://example.com/v1/LICENSE\n"

	tests := []struct {
		name      string
		generated string
		want      string
	}{
		{
			name:      "unknown license and URL retain known metadata for the same version",
			generated: strings.ReplaceAll(strings.ReplaceAll(previous, "License: MIT", "License: Unknown"), "https://example.com/v1/LICENSE", "Unknown"),
			want:      previous,
		},
		{
			name:      "unknown URL retains its known value without changing a newly detected license",
			generated: strings.ReplaceAll(strings.ReplaceAll(previous, "License: MIT", "License: BSD-3-Clause"), "https://example.com/v1/LICENSE", "Unknown"),
			want:      strings.ReplaceAll(previous, "License: MIT", "License: BSD-3-Clause"),
		},
		{
			name:      "known URL changes are kept",
			generated: strings.ReplaceAll(previous, "https://example.com/v1/LICENSE", "https://example.com/v1/COPYING"),
			want:      strings.ReplaceAll(previous, "https://example.com/v1/LICENSE", "https://example.com/v1/COPYING"),
		},
		{
			name:      "new version does not inherit the old URL",
			generated: strings.ReplaceAll(strings.ReplaceAll(previous, "v1.0.0", "v2.0.0"), "https://example.com/v1/LICENSE", "Unknown"),
			want:      strings.ReplaceAll(strings.ReplaceAll(previous, "v1.0.0", "v2.0.0"), "https://example.com/v1/LICENSE", "Unknown"),
		},
		{
			name:      "new dependency keeps Unknown",
			generated: "Header\n\n----------\nModule: example.com/new\nVersion: v1.0.0\nLicense: Unknown\nLicense URL: Unknown\n",
			want:      "Header\n\n----------\nModule: example.com/new\nVersion: v1.0.0\nLicense: Unknown\nLicense URL: Unknown\n",
		},
		{
			name:      "removed dependency stays removed",
			generated: "Header\n",
			want:      "Header\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := preserveKnownLicenses(previous, tt.generated)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}

			if again := preserveKnownLicenses(got, tt.generated); again != got {
				t.Fatalf("second generation changed NOTICE: %q", again)
			}
		})
	}
}

func TestPreserveKnownLicensesWithoutVersion(t *testing.T) {
	for _, version := range []string{"", "Unknown"} {
		previous := "\n----------\nModule: example.com/library\nVersion: " + version + "\nLicense: MIT\nLicense URL: https://example.com/LICENSE\n"

		generated := strings.ReplaceAll(previous, "https://example.com/LICENSE", "Unknown")
		if got := preserveKnownLicenses(previous, generated); got != generated {
			t.Fatalf("unidentified version inherited metadata: %q", got)
		}
	}
}

func TestPreserveKnownLicensesWithoutPreviousReport(t *testing.T) {
	const generated = "\n----------\nModule: example.com/library\nVersion: v1.0.0\nLicense: MIT\nLicense URL: Unknown\n"
	if got := preserveKnownLicenses("", generated); got != generated {
		t.Fatalf("first generation changed: %q", got)
	}
}
