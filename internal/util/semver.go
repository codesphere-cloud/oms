// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"

	"golang.org/x/mod/semver"
)

// InstallVersionAtLeast returns true if the codesphere installVersion is higher or equal to minimum.
func InstallVersionAtLeast(installVersion, minimum string) bool {
	var v string
	for _, prefix := range []string{"codesphere-"} {
		v = strings.TrimPrefix(installVersion, prefix)
	}

	parsedVersion := semver.Canonical(v)
	if parsedVersion == "" {
		return false
	}

	compare := semver.Compare(parsedVersion, minimum) >= 0

	return compare
}
