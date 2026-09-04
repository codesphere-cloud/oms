package util

import (
	"strings"
	"golang.org/x/mod/semver"
)

func InstallVersionAtLeast(installVersion, minimum string) bool {
	for _, prefix := range []string{"codesphere-", "codesphere/"} {
		version = strings.TrimPrefix(installVersion, prefix)
	}

	version = semver.Canonical(version)
	if version == "" {
		return false
	}

	return semver.Compare(version, minimum) >= 0
}
