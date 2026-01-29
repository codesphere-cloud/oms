// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

func Truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
