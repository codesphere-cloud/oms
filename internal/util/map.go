// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"reflect"

	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

// StringSliceToBoolMap converts a slice of strings into a set-style map where
// every element maps to true. A nil slice yields a nil map; a non-nil (possibly
// empty) slice yields a non-nil map.
func StringSliceToBoolMap(items []string) map[string]bool {
	if items == nil {
		return nil
	}
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// DeepMergeMaps recursively merges two maps of string to any, returning a new merged map.
// Values from the src map will overwrite those in dst when there are conflicts, except when both values are maps themselves,
// in which case they will be merged recursively. The original dst map is not modified; a new map is returned.
func DeepMergeMaps(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}

	for key, srcVal := range src {
		srcMap, srcIsMap := srcVal.(map[string]any)
		if !srcIsMap {
			dst[key] = srcVal
			continue
		}

		dstMap, dstIsMap := dst[key].(map[string]any)
		if !dstIsMap || dstMap == nil {
			dstMap = map[string]any{}
		}

		dst[key] = DeepMergeMaps(dstMap, srcMap)
	}

	return dst
}

// MergeChartValues layers override over base like Helm layers values files.
func MergeChartValues(base, override map[string]any) map[string]any {
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
