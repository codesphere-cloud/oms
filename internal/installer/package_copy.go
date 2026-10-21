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
	"github.com/codesphere-cloud/oms/internal/installer/imageref"
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

// ReadPackageArtifacts reads all images and OCI Helm charts from a BOM and builds their
// destination references. Images keep their registry and repository path below dest, so that
// repositories with the same basename cannot collide. Charts keep their repository path alone,
// because that is where an installation looks for them.
func ReadPackageArtifacts(bomPath, dest string) ([]PackageArtifact, error) {
	bomConfig, err := bom.Parse(bomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse BOM: %w", err)
	}

	images := bomConfig.GetContainerImages()
	charts := bomConfig.GetChartRefs()

	artifacts := make([]PackageArtifact, 0, len(images)+len(charts))
	for _, source := range images {
		destination, err := PackageImageDestination(source, dest)
		if err != nil {
			return nil, err
		}

		artifacts = append(artifacts, PackageArtifact{
			Source:      strings.TrimPrefix(source, "oci://"),
			Destination: destination,
		})
	}

	for _, source := range charts {
		destination, err := PackageChartDestination(source, dest)
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

// PackageImageDestination maps a container image below a destination registry or repository
// prefix. The source registry is kept as its own path segment, because that is how an
// installation rewrites the images of its BOM onto the registry it pulls from.
func PackageImageDestination(source, dest string) (string, error) {
	return packageArtifactDestination(source, dest, true)
}

// ChartsRepositoryPath is the repository path the Codesphere Helm charts keep below a mirror they
// are copied into by PackageChartDestination, and so where ArgoCD and pc-applications pull them
// from on a mirrored registry.
const ChartsRepositoryPath = "/codesphere-cloud/charts"

// PackageChartDestination maps an OCI Helm chart below a destination registry or repository
// prefix. An installation does not rewrite its chart references and addresses them by their
// repository path alone, so the source registry is dropped.
func PackageChartDestination(source, dest string) (string, error) {
	return packageArtifactDestination(source, dest, false)
}

// packageArtifactDestination maps a source reference below a destination
// registry or repository prefix while preserving its tag or digest.
func packageArtifactDestination(source, dest string, keepRegistry bool) (string, error) {
	source = strings.TrimPrefix(source, "oci://")

	dest = strings.TrimSuffix(strings.TrimPrefix(dest, "oci://"), "/")
	if dest == "" {
		return "", fmt.Errorf("destination registry must not be empty")
	}

	sourceRef, err := name.ParseReference(source)
	if err != nil {
		return "", fmt.Errorf("invalid package artifact reference %q: %w", source, err)
	}

	repository, tag, digest := imageref.Split(source)

	// A reference carrying both a tag and a digest is copied to its tag, because a manifest pushed
	// under a digest alone leaves the repository without the tag the Helm charts pull by. Its
	// digest keeps resolving either way, the copy does not change the manifest.
	var identifier string

	switch {
	case tag != "":
		identifier = ":" + tag
	case digest != "":
		identifier = "@" + digest
	default:
		// Neither is set, the parsed reference supplies the implicit latest tag.
		identifier = ":" + sourceRef.Identifier()
	}

	repositoryPath := sourceRef.Context().RepositoryStr()
	if keepRegistry {
		repositoryPath = imageref.MirrorRepository(repository)
	}

	candidate := dest + "/" + repositoryPath + identifier

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
