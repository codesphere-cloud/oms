// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
)

// AirgapBundleName is the name of target airgap-bundle for oms to download to
const AirgapBundleName = "k0s-airgap-bundle"

// DownloadAirgapBundle downloads the airgap image bundle of the given version to path,
// unless a file already exists there, and returns its path. An empty path stores the
// bundle in the OMS cache dir under a fixed name, so a cached bundle is found without
// contacting the release API.
func (k *K0s) DownloadAirgapBundle(version, path string, opts DownloadOptions) (string, error) {
	if path == "" {
		cacheDir, err := ensureCacheDir(k.FileWriter, k.Env)
		if err != nil {
			return "", err
		}

		path = filepath.Join(cacheDir, fmt.Sprintf("%s-%s-%s-%s.tar", AirgapBundleName, version, k.Goos, k.Goarch))
	} else if err := k.FileWriter.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for airgap bundle %s: %w", path, err)
	}

	if !opts.Force && k.FileWriter.Exists(path) {
		io.Verbosef(!opts.Quiet, "Using existing airgap bundle %s", path)

		return path, nil
	}

	assetName, err := k.resolveAirgapBundleAssetName(version)
	if err != nil {
		return "", err
	}

	downloadURL := fmt.Sprintf("%s/%s/%s", k0sReleaseURL, version, assetName)
	io.Verbosef(!opts.Quiet, "Downloading k0s airgap bundle from %s", downloadURL)

	if err := downloadToPath(k.FileWriter, k.Http, path, downloadURL, opts.Quiet, 0644); err != nil {
		return "", err
	}

	return path, nil
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

	prefix := fmt.Sprintf("%s-%s-", AirgapBundleName, version)

	i := slices.IndexFunc(release.Assets, func(asset githubReleaseAsset) bool {
		platform, ok := strings.CutPrefix(asset.Name, prefix)
		platform = strings.TrimSuffix(platform, ".tar")

		return ok && (platform == k.Goarch || platform == k.Goos+"-"+k.Goarch)
	})
	if i < 0 {
		return "", fmt.Errorf("no airgap bundle for %s/%s found in k0s release %s", k.Goos, k.Goarch, version)
	}

	return release.Assets[i].Name, nil
}
