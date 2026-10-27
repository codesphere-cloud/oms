// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// releaseBranchVersion matches the version part of release branch names like codesphere-1-114-0.
var releaseBranchVersion = regexp.MustCompile(`^v?(\d+)-(\d+)-(\d+)$`)

// InstallVersionAtLeast returns true if the codesphere installVersion is higher or equal to minimum.
// installVersion can be a release tag (codesphere-v1.114.0) or a release branch name (codesphere-1-114-0).
func InstallVersionAtLeast(installVersion, minimum string) bool {
	v := strings.TrimPrefix(installVersion, "codesphere-")
	v = releaseBranchVersion.ReplaceAllString(v, "v$1.$2.$3")
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}

	parsedVersion := semver.Canonical(v)
	if parsedVersion == "" {
		return false
	}

	return semver.Compare(parsedVersion, minimum) >= 0
}
