// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// releaseBranchVersion matches dash-separated versions like 1-114-0.
var releaseBranchVersion = regexp.MustCompile(`^(\d+)-(\d+)-(\d+)$`)

// InstallVersionAtLeast returns true if the codesphere installVersion is higher or equal to minimum.
func InstallVersionAtLeast(installVersion, minimum string) bool {
	var v string
	for _, prefix := range []string{"codesphere-"} {
		v = strings.TrimPrefix(installVersion, prefix)
	}
	v = releaseBranchVersion.ReplaceAllString(v, "v$1.$2.$3")

	parsedVersion := semver.Canonical(v)
	if parsedVersion == "" {
		return false
	}

	compare := semver.Compare(parsedVersion, minimum) >= 0

	return compare
}
