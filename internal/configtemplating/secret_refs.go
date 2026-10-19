// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package configtemplating

import (
	"fmt"
	"slices"
	"strings"
)

// ValidateCodesphereSecretRefs rejects `secret` calls in the codesphere block of a config
// rendered with RenderSecretRefs. Their values reach the Codesphere chart's ExternalSecrets in
// plaintext. Dev releases are exempt; only they set codesphere.releaseName.
func ValidateCodesphereSecretRefs(config map[string]any) error {
	codesphere, ok := config["codesphere"].(map[string]any)
	if !ok {
		return nil
	}

	if releaseName, _ := codesphere["releaseName"].(string); releaseName != "" {
		return nil
	}

	paths := secretRefPaths("codesphere", codesphere)
	if len(paths) == 0 {
		return nil
	}

	slices.Sort(paths)

	return fmt.Errorf(
		"secret templates are not supported in the codesphere block, found at %s",
		strings.Join(paths, ", "),
	)
}

func secretRefPaths(path string, value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.Contains(typed, secretRefMarker) {
			return []string{path}
		}
	case map[string]any:
		paths := []string{}
		for key, child := range typed {
			paths = append(paths, secretRefPaths(fmt.Sprintf("%s.%s", path, key), child)...)
		}

		return paths
	case []any:
		paths := []string{}
		for i, child := range typed {
			paths = append(paths, secretRefPaths(fmt.Sprintf("%s[%d]", path, i), child)...)
		}

		return paths
	}

	return nil
}
