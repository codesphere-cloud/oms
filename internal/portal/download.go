// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"fmt"
	"os"

	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// DownloadOptions configures DownloadAndVerifyBuild.
type DownloadOptions struct {
	// Resume appends to an existing partial download at destination instead
	// of always starting over.
	Resume bool
	// Quiet suppresses download progress output.
	Quiet bool
}

// DownloadAndVerifyBuild downloads the named artifact from build to destination
// and verifies its checksum. It is shared by the download and copy package
// commands so their download and verify behavior stays in sync.
func DownloadAndVerifyBuild(p Portal, fileWriter intutil.FileIO, product Product, build Build, filename, destination string, opts DownloadOptions) error {
	download, err := build.GetBuildForDownload(filename)
	if err != nil {
		return fmt.Errorf("failed to find artifact in package: %w", err)
	}

	out, err := openDestination(fileWriter, destination, opts.Resume)
	if err != nil {
		return fmt.Errorf("failed to open pckage destination file: %w", err)
	}
	defer intutil.CloseFileIgnoreError(out)

	fileSize := 0
	if opts.Resume {
		if fileInfo, statErr := out.Stat(); statErr == nil {
			fileSize = int(fileInfo.Size())
		}
	}

	if err := p.DownloadBuildArtifact(product, download, out, fileSize, opts.Quiet); err != nil {
		return fmt.Errorf("failed to download build: %w", err)
	}

	verifyFile, err := fileWriter.Open(destination)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", destination, err)
	}
	defer intutil.CloseFileIgnoreError(verifyFile)

	if err := p.VerifyBuildArtifactDownload(verifyFile, download); err != nil {
		return fmt.Errorf("failed to verify artifact: %w", err)
	}

	return nil
}

func openDestination(fileWriter intutil.FileIO, destination string, resume bool) (*os.File, error) {
	if resume {
		if out, err := fileWriter.OpenAppend(destination); err == nil {
			return out, nil
		}
	}

	file, err := fileWriter.Create(destination)
	if err != nil {
		return nil, fmt.Errorf("failed to open file at %q: %w", destination, err)
	}
	return file, nil
}
