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
// builds their destination references. The original registry and repository path are kept
// below dest so repositories with the same basename cannot collide.
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

// registrySegmentReplacer flattens a registry into a single path segment.
var registrySegmentReplacer = strings.NewReplacer(".", "_", ":", "_")

// splitReference separates a reference into its repository, tag and digest. A reference can carry
// both a tag and a digest, which the installer keeps and the reference parser does not.
func splitReference(source string) (repository, tag, digest string) {
	repository = source
	if at := strings.Index(repository, "@"); at != -1 {
		digest = repository[at+1:]
		repository = repository[:at]
	}

	if colon := strings.LastIndex(repository, ":"); colon > strings.LastIndex(repository, "/") {
		tag = repository[colon+1:]
		repository = repository[:colon]
	}

	return repository, tag, digest
}

// destinationRepository returns the repository path a reference keeps below the destination. The
// installer rewrites the images of a BOM onto another registry by moving the source registry into
// the first path segment with its dots and colons replaced by underscores, so that
// ghcr.io/codesphere-cloud/api ends up as <server>/ghcr_io/codesphere-cloud/api. The same layout
// is built here, because an installation only finds the mirrored artifacts if both sides name them
// identically. The repository is split as a plain string rather than parsed, since the installer
// reads the leading segment of a reference without a registry as one, keeping alpine/kubectl below
// alpine instead of below docker.io.
func destinationRepository(repository string) string {
	registry, path, found := strings.Cut(repository, "/")

	registry = registrySegmentReplacer.Replace(registry)
	if !found {
		return registry
	}

	return registry + "/" + path
}

// PackageImageDestination maps a container image below a destination registry or repository
// prefix. The source registry is kept as its own path segment, because that is how an
// installation rewrites the images of its BOM onto the registry it pulls from.
func PackageImageDestination(source, dest string) (string, error) {
	return packageArtifactDestination(source, dest, true)
}

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

	repository, tag, digest := splitReference(source)

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
		repositoryPath = destinationRepository(repository)
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
