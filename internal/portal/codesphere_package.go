// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"fmt"
	"os"
	"path/filepath"

	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// EnsureCodespherePackage resolves the configured build and delegates archive
// downloads, resuming, and checksum verification to the shared downloader.
func EnsureCodespherePackage(p Portal, workdir, version, hash, filename string, quiet bool) (string, error) {
	if version == "" {
		return "", fmt.Errorf("codesphere package version is required")
	}

	build, err := p.GetBuild(CodesphereProduct, version, hash)
	if err != nil {
		return "", fmt.Errorf("failed to get codesphere package: %w", err)
	}

	destination := filepath.Join(workdir, build.BuildPackageFilename(filename))

	if err := os.MkdirAll(workdir, 0755); err != nil {
		return "", fmt.Errorf("failed to create package workdir: %w", err)
	}

	if err := DownloadAndVerifyBuild(p, intutil.NewFilesystemWriter(), CodesphereProduct, build, filename, destination, DownloadOptions{Resume: true, Quiet: quiet}); err != nil {
		return "", fmt.Errorf("failed to download codesphere package: %w", err)
	}

	return destination, nil
}
