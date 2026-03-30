// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// downloadBinary downloads a binary from downloadURL into workdir/binaryName.
// It handles workdir creation, existing binary checks, file creation, download, and chmod.
func downloadBinary(fw util.FileIO, http portal.Http, workdir, binaryName, downloadURL string, force bool, quiet bool) (string, error) {
	if err := fw.MkdirAll(workdir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workdir: %w", err)
	}

	binaryPath := filepath.Join(workdir, binaryName)
	if fw.Exists(binaryPath) && !force {
		return "", fmt.Errorf("%s binary already exists at %s. Use --force to overwrite", binaryName, binaryPath)
	}

	dstFile, err := fw.Create(binaryPath)
	if err != nil {
		return "", fmt.Errorf("failed to create %s binary file: %w", binaryName, err)
	}
	defer util.CloseFileIgnoreError(dstFile)

	if err := http.Download(downloadURL, dstFile, quiet); err != nil {
		return "", fmt.Errorf("failed to download %s binary: %w", binaryName, err)
	}

	if err := fw.Chmod(binaryPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make %s binary executable: %w", binaryName, err)
	}

	return binaryPath, nil
}
