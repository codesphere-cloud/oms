// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
)

const (
	// AirgapBundleName is the name of target airgap-bundle for oms to download to
	AirgapBundleName = "k0s-airgap-bundle"

	// AirgapImagesDir is the k0s data dir from which worker nodes import the
	// images of an airgap bundle. Controllers that do not run a worker ignore it.
	AirgapImagesDir = "/var/lib/k0s/images"
)

// AirgapOptions describes an airgapped installation. Enabled marks the installation
// as airgapped and BundlePath is the local airgap image bundle that k0sctl uploads
// to the worker nodes.
type AirgapOptions struct {
	Enabled    bool
	BundlePath string
}

// EnsureAirgapBundle makes sure the airgap image bundle of the given version is
// available in the OMS cache dir and returns its path.
func (k *K0s) EnsureAirgapBundle(version string, opts DownloadOptions) (string, error) {
	cacheDir, err := ensureCacheDir(k.FileWriter, k.Env)
	if err != nil {
		return "", err
	}

	return k.ensureAirgapBundle(version, cacheDir, opts)
}

// ensureAirgapBundle downloads the airgap image bundle into cacheDir unless a bundle
// of that version is already cached there, and returns its path.
func (k *K0s) ensureAirgapBundle(version, cacheDir string, opts DownloadOptions) (string, error) {
	cachePath, err := k.airgapBundleCachePath(version, cacheDir)
	if err != nil {
		return "", err
	}

	if k.FileWriter.Exists(cachePath) && !opts.Force {
		io.Verbosef(!opts.Quiet, "Using cached airgap bundle %s", cachePath)

		return cachePath, nil
	}

	downloadURL := releaseAssetURL(k0sReleaseURL, version, filepath.Base(cachePath))
	io.Verbosef(!opts.Quiet, "Downloading k0s airgap bundle from %s", downloadURL)

	if err := downloadToPath(k.FileWriter, k.Http, cachePath, downloadURL, opts.Quiet); err != nil {
		return "", err
	}

	return cachePath, nil
}

// airgapBundleCachePath prefers an already cached bundle of the given version over
// resolving the release metadata, so airgapped installations can stay offline.
func (k *K0s) airgapBundleCachePath(version, cacheDir string) (string, error) {
	if entries, err := k.FileWriter.ReadDir(cacheDir); err == nil {
		for _, entry := range entries {
			if k.isAirgapBundleFor(entry.Name(), version) {
				return filepath.Join(cacheDir, entry.Name()), nil
			}
		}
	}

	assetName, err := k.resolveAirgapBundleAssetName(version)
	if err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, assetName), nil
}

// resolveAirgapBundleAssetName returns the release asset name of the airgap image
// bundle of the given version and architecture.
//
// k0s changed the asset naming scheme in v1.36, from
// "k0s-airgap-bundle-<version>-<arch>" to
// "k0s-airgap-bundle-<version>-<os>-<arch>.tar", so the name is resolved from the
// release metadata instead of being constructed.
func (k *K0s) resolveAirgapBundleAssetName(version string) (string, error) {
	responseBody, err := k.Http.Get(fmt.Sprintf("%s/%s", k0sReleaseAPIURL, version))
	if err != nil {
		return "", fmt.Errorf("failed to fetch k0s release %s: %w", version, err)
	}

	var release githubRelease
	if err := json.Unmarshal(responseBody, &release); err != nil {
		return "", fmt.Errorf("failed to parse k0s release %s: %w", version, err)
	}

	for _, asset := range release.Assets {
		if k.isAirgapBundleFor(asset.Name, version) {
			return asset.Name, nil
		}
	}

	return "", fmt.Errorf("no airgap bundle for %s/%s found in k0s release %s", k.Goos, k.Goarch, version)
}

// isAirgapBundleFor reports whether name is the airgap image bundle asset of the
// given version that matches the configured OS and architecture. It accepts both the
// legacy and the current upstream naming scheme.
func (k *K0s) isAirgapBundleFor(name, version string) bool {
	platform, ok := strings.CutPrefix(name, fmt.Sprintf("%s-%s-", AirgapBundleName, version))
	if !ok {
		return false
	}

	platform = strings.TrimSuffix(platform, ".tar")

	return platform == k.Goarch || platform == k.Goos+"-"+k.Goarch
}
