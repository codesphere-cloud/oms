// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// EnsureAirgapBundle makes sure the airgap image bundle of the given version is
// available in the OMS cache dir and returns its path.
func (k *K0s) EnsureAirgapBundle(version string, opts DownloadOptions) (string, error) {
	cacheDir, err := ensureCacheDir(k.FileWriter, k.Env)
	if err != nil {
		return "", err
	}

	cachePath, found := k.cachedAirgapBundle(cacheDir, version)
	if !found {
		assetName, err := k.resolveAirgapBundleAssetName(version)
		if err != nil {
			return "", err
		}

		cachePath = filepath.Join(cacheDir, assetName)
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

// cachedAirgapBundle returns the path of the cached airgap bundle of the given
// version, if there is one. An unreadable cache is not an error: the release
// metadata then decides which bundle to look for.
func (k *K0s) cachedAirgapBundle(cacheDir, version string) (string, bool) {
	entries, err := k.FileWriter.ReadDir(cacheDir)
	if err != nil {
		return "", false
	}

	i := slices.IndexFunc(entries, func(entry os.DirEntry) bool { return k.isAirgapBundleFor(entry.Name(), version) })
	if i < 0 {
		return "", false
	}

	return filepath.Join(cacheDir, entries[i].Name()), true
}

// resolveAirgapBundleAssetName returns the release asset name of the airgap image
// bundle of the given version and architecture.
//
// k0s changed the asset naming scheme in v1.36, from
// "k0s-airgap-bundle-<version>-<arch>" to
// "k0s-airgap-bundle-<version>-<os>-<arch>.tar", so the name is resolved from the
// release metadata instead of being constructed.
func (k *K0s) resolveAirgapBundleAssetName(version string) (string, error) {
	release, err := getGitHubRelease(k.Http, k0sReleaseAPIURL+"/"+version, "k0s release "+version)
	if err != nil {
		return "", err
	}

	i := slices.IndexFunc(release.Assets, func(asset githubReleaseAsset) bool { return k.isAirgapBundleFor(asset.Name, version) })
	if i < 0 {
		return "", fmt.Errorf("no airgap bundle for %s/%s found in k0s release %s", k.Goos, k.Goarch, version)
	}

	return release.Assets[i].Name, nil
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
