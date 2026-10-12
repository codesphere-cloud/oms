// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"fmt"
	"log"

	"github.com/codesphere-cloud/oms/internal/installer"
)

// resolveK0sVersion returns the requested k0s version, or the latest version when
// none was requested, and logs the version that is used.
func resolveK0sVersion(k0s installer.K0sManager, version string) (string, error) {
	if version == "" {
		latestVersion, err := k0s.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest k0s version: %w", err)
		}

		version = latestVersion
	}

	log.Printf("Using k0s version %s", version)

	return version, nil
}
