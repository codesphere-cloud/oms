// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func TestCodesphereConfigIsEnabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  files.CodesphereConfig
		enabled bool
	}{
		{name: "internal", config: files.CodesphereConfig{Internal: []string{"other", "target"}}, enabled: true},
		{name: "preview", config: files.CodesphereConfig{Preview: map[string]bool{"target": true}}, enabled: true},
		{name: "feature", config: files.CodesphereConfig{Features: map[string]bool{"target": true}}, enabled: true},
		{name: "disabled", config: files.CodesphereConfig{Preview: map[string]bool{"target": false}, Features: map[string]bool{"target": false}}},
		{name: "unrelated", config: files.CodesphereConfig{Internal: []string{"other"}, Preview: map[string]bool{"other": true}, Features: map[string]bool{"other": true}}},
		{name: "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.config.IsEnabled("target"); got != tc.enabled {
				t.Errorf("IsEnabled(target) = %t, want %t", got, tc.enabled)
			}
		})
	}
}
