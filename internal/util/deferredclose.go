// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

func IgnoreError(fn func() error) {
	_ = fn()
}
