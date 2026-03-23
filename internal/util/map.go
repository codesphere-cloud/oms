// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

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
