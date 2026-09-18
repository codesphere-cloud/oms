// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	// DefaultK0sVersion is the currently verified k0s version
	// Use of newer versions should work in most cases but can't be guaranteed
	DefaultK0sVersion = "v1.31.14+k0s.0"

	// GitHubReleaseURL is the github release page for k0s
	GitHubReleaseURL = "https://github.com/k0sproject/k0s/releases/download"

	// k0sReleaseAPIURL lists the assets of a k0s release
	k0sReleaseAPIURL = "https://api.github.com/repos/k0sproject/k0s/releases/tags"

	// BinaryName is the name of target binary for oms to download to
	BinaryName = "k0s"

	// AirgapBundleName is the name of target airgap-bundle for oms to download to
	AirgapBundleName = "k0s-airgap-bundle"

	// AirgapImagesDir is the k0s data dir from which worker nodes import the
	// images of an airgap bundle. Controllers that do not run a worker ignore it.
	AirgapImagesDir = "/var/lib/k0s/images"
)

//mockery:generate: true
type K0sManager interface {
	GetLatestVersion() (string, error)
	Download(version string, force bool, quiet bool, airgapped bool) (string, error)
	EnsureAirgapBundle(version string, force bool, quiet bool) (string, error)
}

type k0sRelease struct {
	Assets []k0sReleaseAsset `json:"assets"`
}

type k0sReleaseAsset struct {
	Name string `json:"name"`
}

type K0s struct {
	Env        env.Env
	Http       portal.Http
	FileWriter util.FileIO
	Goos       string
	Goarch     string
}

func NewK0s(hw portal.Http, env env.Env, fw util.FileIO) K0sManager {
	return &K0s{
		Env:        env,
		Http:       hw,
		FileWriter: fw,
		Goos:       runtime.GOOS,
		Goarch:     runtime.GOARCH,
	}
}

func (k *K0s) GetLatestVersion() (string, error) {
	versionBytes, err := k.Http.Get("https://docs.k0sproject.io/stable.txt")
	if err != nil {
		return "", fmt.Errorf("failed to fetch version info: %w", err)
	}

	version := strings.TrimSpace(string(versionBytes))
	if version == "" {
		return "", fmt.Errorf("version info is empty, cannot proceed with download")
	}

	return version, nil
}

// Download downloads the k0s binary for the specified version and saves it to the OMS cache dir.
func (k *K0s) Download(version string, force, quiet, airgapped bool) (string, error) {
	if k.Goos != "linux" || k.Goarch != "amd64" {
		return "", fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", k.Goos, k.Goarch)
	}

	log.Printf("Downloading k0s version %s", version)

	cacheDir, err := k.Env.GetOmsCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine cache directory: %w", err)
	}

	if err := k.FileWriter.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workdir: %w", err)
	}

	path, err := k.downloadBinary(version, cacheDir, force, quiet)
	if err != nil {
		return "", fmt.Errorf("failed to download k0s binary: %w", err)
	}

	if airgapped {
		bundlePath, bundleErr := k.ensureAirgapBundle(version, cacheDir, force, quiet)
		if bundleErr != nil {
			return "", bundleErr
		}

		log.Printf("k0s airgap bundle downloaded to '%s'", bundlePath)
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", path)

	return path, nil
}

// EnsureAirgapBundle makes sure the airgap image bundle of the given version is
// available in the OMS cache dir and returns its path.
func (k *K0s) EnsureAirgapBundle(version string, force, quiet bool) (string, error) {
	cacheDir, err := k.Env.GetOmsCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine cache directory: %w", err)
	}

	if err := k.FileWriter.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workdir: %w", err)
	}

	return k.ensureAirgapBundle(version, cacheDir, force, quiet)
}

// downloadBinary fetches the k0s binary for the given version from the k0s GitHub
// releases and stores it as "k0s" in cacheDir, returning the path to it.
// If a binary is already cached and force is false, the cached binary is reused as
// long as its version matches; otherwise it is replaced by a fresh download.
func (k *K0s) downloadBinary(version, cacheDir string, force, quiet bool) (string, error) {
	cachePath := filepath.Join(cacheDir, BinaryName)
	if k.FileWriter.Exists(cachePath) && !force {
		cachedVersion, versionErr := localBinaryVersion(cachePath)
		if versionErr == nil && cachedVersion == version {
			io.Verbosef(!quiet, "Using cached k0s %s at %s", version, cachePath)
			return cachePath, nil
		}

		replaceReason := fmt.Sprintf("Cached k0s version %s does not match requested version %s; replacing it", cachedVersion, version)
		if versionErr != nil {
			replaceReason = "Cached k0s version could not be determined: " + versionErr.Error()
		}

		io.Verbosef(!quiet, "Replacing existing k0s binary: %s", replaceReason)
	}

	downloadURL := fmt.Sprintf("%s/%s/k0s-%s-%s", GitHubReleaseURL, version, version, k.Goarch)

	path, err := downloadBinaryToPath(k.FileWriter, k.Http, cachePath, BinaryName, downloadURL, quiet)
	if err != nil {
		return "", err
	}

	return path, err
}

// ensureAirgapBundle downloads the airgap image bundle into cacheDir unless a bundle
// of that version is already cached there, and returns its path.
func (k *K0s) ensureAirgapBundle(version, cacheDir string, force, quiet bool) (string, error) {
	cachePath, err := k.airgapBundleCachePath(version, cacheDir)
	if err != nil {
		return "", err
	}

	if k.FileWriter.Exists(cachePath) && !force {
		io.Verbosef(!quiet, "Using cached airgap bundle %s", cachePath)

		return cachePath, nil
	}

	downloadURL := fmt.Sprintf("%s/%s/%s", GitHubReleaseURL, version, filepath.Base(cachePath))
	io.Verbosef(!quiet, "Downloading k0s airgap bundle from %s", downloadURL)

	if err := downloadToPath(k.FileWriter, k.Http, cachePath, downloadURL, quiet); err != nil {
		return "", err
	}

	return cachePath, nil
}

// airgapBundleCachePath returns the path of the airgap image bundle of the given
// version in cacheDir. An already cached bundle is preferred, so airgapped
// installations do not have to resolve the release metadata over the network.
func (k *K0s) airgapBundleCachePath(version, cacheDir string) (string, error) {
	if cachedPath, found := k.cachedAirgapBundlePath(version, cacheDir); found {
		return cachedPath, nil
	}

	assetName, err := k.resolveAirgapBundleAssetName(version)
	if err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, assetName), nil
}

// cachedAirgapBundlePath looks for an already downloaded airgap bundle of the given
// version and architecture in cacheDir.
func (k *K0s) cachedAirgapBundlePath(version, cacheDir string) (string, bool) {
	entries, err := k.FileWriter.ReadDir(cacheDir)
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		if k.isAirgapBundleFor(entry.Name(), version) {
			return filepath.Join(cacheDir, entry.Name()), true
		}
	}

	return "", false
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

	var release k0sRelease
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
	prefix := fmt.Sprintf("%s-%s-", AirgapBundleName, version)
	if !strings.HasPrefix(name, prefix) {
		return false
	}

	platform := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".tar")

	return platform == k.Goarch || platform == fmt.Sprintf("%s-%s", k.Goos, k.Goarch)
}
