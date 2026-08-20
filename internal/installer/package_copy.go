// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/logs"
	"github.com/google/go-containerregistry/pkg/name"
)

// PackageArtifact describes one image or OCI Helm chart transfer.
type PackageArtifact struct {
	Source      string
	Destination string
}

// ArtifactCopier copies an image or OCI artifact between registries.
type ArtifactCopier interface {
	Copy(ctx context.Context, source, destination string) error
}

// CraneArtifactCopier uses go-containerregistry's crane package for transfers.
type CraneArtifactCopier struct {
	Insecure bool
}

// Copy transfers one remote image or OCI artifact with crane.
func (c *CraneArtifactCopier) Copy(ctx context.Context, source, destination string) error {
	options := []crane.Option{crane.WithContext(ctx), crane.WithNondistributable()}
	if c.Insecure {
		options = append(options, crane.Insecure)
	}
	if err := crane.Copy(source, destination, options...); err != nil {
		return fmt.Errorf("crane copy failed: %w", err)
	}

	return nil
}

// ReadPackageArtifacts reads all images and OCI Helm charts from a BOM and
// builds their destination references. The original repository path is kept
// below dest so repositories with the same basename cannot collide.
func ReadPackageArtifacts(bomPath, dest string) ([]PackageArtifact, error) {
	bomConfig, err := bom.Parse(bomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse BOM: %w", err)
	}

	references := bomConfig.GetOCIArtifacts()

	artifacts := make([]PackageArtifact, 0, len(references))
	for _, source := range references {
		destination, err := PackageArtifactDestination(source, dest)
		if err != nil {
			return nil, err
		}

		artifacts = append(artifacts, PackageArtifact{
			Source:      strings.TrimPrefix(source, "oci://"),
			Destination: destination,
		})
	}

	return artifacts, nil
}

// PackageArtifactDestination maps a source reference below a destination
// registry or repository prefix while preserving its tag or digest.
func PackageArtifactDestination(source, dest string) (string, error) {
	source = strings.TrimPrefix(source, "oci://")

	dest = strings.TrimSuffix(strings.TrimPrefix(dest, "oci://"), "/")
	if dest == "" {
		return "", fmt.Errorf("destination registry must not be empty")
	}

	sourceRef, err := name.ParseReference(source)
	if err != nil {
		return "", fmt.Errorf("invalid package artifact reference %q: %w", source, err)
	}

	separator := ":"
	if _, ok := sourceRef.(name.Digest); ok {
		separator = "@"
	}

	candidate := dest + "/" + sourceRef.Context().RepositoryStr() + separator + sourceRef.Identifier()

	destinationRef, err := name.ParseReference(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid destination reference %q: %w", candidate, err)
	}

	return destinationRef.Name(), nil
}

// CopyPackageArtifacts transfers the prepared package artifacts in order,
// printing a single updating progress bar. Crane's own log output is
// captured rather than written to the terminal, so it doesn't clutter the
// progress bar; it's only surfaced if a copy fails.
func CopyPackageArtifacts(ctx context.Context, copier ArtifactCopier, artifacts []PackageArtifact) error {
	total := len(artifacts)
	start := time.Now()

	var craneOutput bytes.Buffer
	logs.Warn.SetOutput(&craneOutput)
	logs.Progress.SetOutput(&craneOutput)
	defer func() {
		logs.Warn.SetOutput(io.Discard)
		logs.Progress.SetOutput(io.Discard)
	}()

	for i, artifact := range artifacts {
		craneOutput.Reset()
		fmt.Printf("\r\033[2K%3d%% (%d/%d) %s %s -> %s", i*100/max(total, 1), i, total, time.Since(start).Round(time.Second), artifact.Source, artifact.Destination)

		if err := copier.Copy(ctx, artifact.Source, artifact.Destination); err != nil {
			fmt.Println()
			if craneOutput.Len() > 0 {
				fmt.Fprint(os.Stderr, craneOutput.String())
			}

			return fmt.Errorf("failed to copy %s to %s: %w", artifact.Source, artifact.Destination, err)
		}
	}

	fmt.Printf("\r\033[2KCopied %d artifacts in %s\n", total, time.Since(start).Round(time.Second))

	return nil
}
