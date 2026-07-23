// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

// AppendUnique appends values that are not already present, preserving the
// order of the original slice and the additions.
func AppendUnique[T comparable](values []T, additions ...T) []T {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
}
