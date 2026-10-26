// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values

import (
	"fmt"

	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

// mergeOverride layers the override over base like Helm layers values files.
func mergeOverride(base, override map[string]any) map[string]any {
	return loader.MergeMaps(base, stringKeys(override).(map[string]any))
}

// stringKeys copies maps and slices so the config is never changed through the values. YAML maps
// with non-string keys, like plan ids, become string keyed as Helm reads them, so they merge with
// the built values.
func stringKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, v := range typed {
			copied[key] = stringKeys(v)
		}

		return copied
	case map[any]any:
		copied := make(map[string]any, len(typed))
		for key, v := range typed {
			copied[fmt.Sprint(key)] = stringKeys(v)
		}

		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, v := range typed {
			copied[i] = stringKeys(v)
		}

		return copied
	default:
		return value
	}
}
