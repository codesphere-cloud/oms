package util

import (
	"strings"

	"golang.org/x/mod/semver"
)

// InstallVersionAtLeast returns true if the codesphere installVersion is higher than minimum.
func InstallVersionAtLeast(installVersion, minimum string) bool {
	var version string
	for _, prefix := range []string{"codesphere-", "codesphere/"} {
		version = strings.TrimPrefix(installVersion, prefix)
	}

	version = semver.Canonical(version)
	if version == "" {
		return false
	}

	return semver.Compare(version, minimum) >= 0
}
