// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

func downloadBinaryToPath(fw util.FileIO, http portal.Http, binaryPath, binaryName, downloadURL string, quiet bool) (string, error) {
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

func localBinaryVersion(binaryPath string) (string, error) {
	output, err := util.RunCommandWithOutput(binaryPath, []string{"version"}, "")
	if err != nil {
		return "", fmt.Errorf("failed to get version of application %s: %w", binaryPath, err)
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if version, found := strings.CutPrefix(line, "version:"); found {
			return strings.TrimSpace(version), nil
		}

		if line != "" {
			return line, nil
		}
	}

	return "", fmt.Errorf("version output is empty")
}
