// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bytes"
	"context"
	"errors"
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
//
//mockery:generate: true
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

// ReadPackageArtifacts prefers the package's bom-index.json, falling back to
// the legacy BOM when the index is absent. Codesphere images use the admission policy
// layout; other images keep their source registry below dest to avoid collisions.
// Charts keep their repository path alone,
// because that is where an installation looks for them.
func ReadPackageArtifacts(packageManager PackageManager, dest string) ([]PackageArtifact, error) {
	bomConfig, err := readPackageBOM(packageManager)
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

// readPackageBOM prefers the package index, falling back only
// when the index is absent so that broken indexes do not silently omit artifacts.
func readPackageBOM(packageManager PackageManager) (packageBOM, error) {
	fileIO := packageManager.FileIO()
	indexPath := packageManager.GetDependencyPath(bom.IndexFilename)

	data, err := fileIO.ReadFile(indexPath)
	if err == nil {
		config, err := bom.ParseIndex(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse BOM index %q: %w", indexPath, err)
		}

		return config, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to read BOM index %q: %w", indexPath, err)
	}

	return readLegacyBOM(packageManager)
}

func readLegacyBOM(packageManager PackageManager) (*bom.Config, error) {
	bomPath := packageManager.GetDependencyPath(bom.Filename)

	data, err := packageManager.FileIO().ReadFile(bomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read BOM file %q: %w", bomPath, err)
	}

	config, err := bom.ParseJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse BOM file %q: %w", bomPath, err)
	}

	return config, nil
}

// packageBOM exposes the artifact references shared by legacy BOMs and v3 indexes.
type packageBOM interface {
	GetContainerImages() []string
	GetChartRefs() []string
	GetCodesphereContainerImages() (map[string]string, error)
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

// destinationRepository keeps Codesphere images at the same repository path used by
// the admission policy. Other images retain their flattened source registry segment,
// matching the existing BOM image layout.
func destinationRepository(repository string) string {
	if strings.HasPrefix(repository, codesphereImagePrefix) {
		return strings.TrimPrefix(repository, DefaultRegistryURL+"/")
	}

	registry, path, found := strings.Cut(repository, "/")

	registry = registrySegmentReplacer.Replace(registry)
	if !found {
		return registry
	}

	return registry + "/" + path
}

// PackageImageDestination maps a container image below a destination registry or repository
// prefix. Codesphere images omit the source registry to match admission rewriting;
// other images retain their source registry as a flattened path segment.
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
