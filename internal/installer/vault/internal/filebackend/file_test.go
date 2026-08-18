// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package filebackend

import "testing"

func TestParseUnwrapsSOPSWholeFileData(t *testing.T) {
	data, err := Parse([]byte("data: |\n    secrets:\n        - name: foo\n          fields:\n            password: bar\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(data.Secrets) != 1 || data.Secrets[0].Name != "foo" || data.Secrets[0].Fields.Password != "bar" {
		t.Fatalf("Parse() = %#v", data)
	}
}

func TestUnwrapSOPSDataLeavesOtherDocumentsUnchanged(t *testing.T) {
	tests := map[string]string{
		"plain vault":        "secrets:\n    - name: foo\n",
		"empty document":     "",
		"multiple root keys": "data: some-value\nsops:\n  key: val\n",
		"invalid YAML":       "not: valid: yaml: [[",
		"non-scalar data":    "data:\n  nested: value\n",
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if got := string(unwrapSOPSData([]byte(input))); got != input {
				t.Fatalf("unwrapSOPSData() = %q, want %q", got, input)
			}
		})
	}
}
