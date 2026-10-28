// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"

	"golang.org/x/mod/semver"
)

// CodesphereRelease normalizes a plain, "v"-prefixed or portal ("codesphere-v") Codesphere
// version to a "v"-prefixed semver. ok is false for versions that are no release, such as
// branch names.
func CodesphereRelease(version string) (release string, ok bool) {
	release = "v" + strings.TrimPrefix(strings.TrimPrefix(version, "codesphere-"), "v")

	return release, semver.IsValid(release)
}

// InstallVersionAtLeast returns true if the codesphere installVersion is higher or equal to minimum.
func InstallVersionAtLeast(installVersion, minimum string) bool {
	release, ok := CodesphereRelease(installVersion)

	return ok && semver.Compare(release, minimum) >= 0
}
