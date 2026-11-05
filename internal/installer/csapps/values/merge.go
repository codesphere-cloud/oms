// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values

import (
	"fmt"
	"reflect"

	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

// MergeOverride layers the override over base like Helm layers values files.
func MergeOverride(base, override map[string]any) map[string]any {
	return loader.MergeMaps(base, copyWithStringKeys(override).(map[string]any))
}

// copyWithStringKeys copies maps and slices of any type so the config is never changed through the
// values. Maps become map[string]any, the only map type MergeMaps merges into: Go-built overrides
// use typed maps, and YAML maps with non-string keys, like plan ids, are string keyed in Helm.
func copyWithStringKeys(value any) any {
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Map:
		copied := make(map[string]any, v.Len())

		iter := v.MapRange()
		for iter.Next() {
			copied[fmt.Sprint(iter.Key().Interface())] = copyWithStringKeys(iter.Value().Interface())
		}

		return copied
	case reflect.Slice:
		copied := make([]any, v.Len())
		for i := range v.Len() {
			copied[i] = copyWithStringKeys(v.Index(i).Interface())
		}

		return copied
	default:
		return value
	}
}
