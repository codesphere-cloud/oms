// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

// mergeValues layers override over base like Helm layers values files: maps merge key by key, a null
// removes the key, and anything else replaces it. base is changed in place.
func mergeValues(base, override map[string]any) map[string]any {
	for key, value := range override {
		if value == nil {
			delete(base, key)
			continue
		}

		overrideMap, overrideIsMap := value.(map[string]any)

		baseMap, baseIsMap := base[key].(map[string]any)
		if overrideIsMap && baseIsMap {
			base[key] = mergeValues(baseMap, overrideMap)
			continue
		}

		base[key] = deepCopy(value)
	}

	return base
}

// deepCopy copies maps and slices so values taken from the config are never changed through the
// built values.
func deepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, v := range typed {
			copied[key] = deepCopy(v)
		}

		return copied
	case map[any]any:
		copied := make(map[any]any, len(typed))
		for key, v := range typed {
			copied[key] = deepCopy(v)
		}

		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, v := range typed {
			copied[i] = deepCopy(v)
		}

		return copied
	default:
		return value
	}
}
