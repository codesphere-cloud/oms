// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testutil

import "os/exec"

func SopsAndAgeAvailable() bool {
	if _, err := exec.LookPath("sops"); err != nil {
		return false
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		return false
	}
	return true
}
