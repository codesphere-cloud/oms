// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"

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
)

//mockery:generate: true
type K0sManager interface {
	GetLatestVersion() (string, error)
	Download(version string, opts DownloadOptions) (string, error)
	EnsureAirgapBundle(version string, opts DownloadOptions) (string, error)
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
// When the airgap option is set, the airgap image bundle of that version is downloaded as well.
func (k *K0s) Download(version string, opts DownloadOptions) (string, error) {
	if k.Goos != "linux" || k.Goarch != "amd64" {
		return "", fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", k.Goos, k.Goarch)
	}

	log.Printf("Downloading k0s version %s", version)

	cacheDir, err := ensureCacheDir(k.FileWriter, k.Env)
	if err != nil {
		return "", err
	}

	path, err := k.downloadBinary(version, cacheDir, opts)
	if err != nil {
		return "", fmt.Errorf("failed to download k0s binary: %w", err)
	}

	if opts.Airgapped {
		bundlePath, bundleErr := k.ensureAirgapBundle(version, cacheDir, opts)
		if bundleErr != nil {
			return "", fmt.Errorf("failed to download k0s airgap bundle: %w", bundleErr)
		}

		log.Printf("k0s airgap bundle downloaded to '%s'", bundlePath)
	}

	return path, nil
}

// downloadBinary fetches the k0s binary for the given version from the k0s GitHub
// releases and stores it as "k0s" in cacheDir, returning the path to it.
// If a binary is already cached and force is false, the cached binary is reused as
// long as its version matches; otherwise it is replaced by a fresh download.
func (k *K0s) downloadBinary(version, cacheDir string, opts DownloadOptions) (string, error) {
	cachePath := filepath.Join(cacheDir, BinaryName)
	if cachedPath, cached := reuseCachedBinary(k.FileWriter, cachePath, version, BinaryName, opts.Force, opts.Quiet); cached {
		return cachedPath, nil
	}

	downloadURL := k.releaseAssetURL(version, fmt.Sprintf("%s-%s-%s", BinaryName, version, k.Goarch))

	path, err := downloadBinaryToPath(k.FileWriter, k.Http, cachePath, BinaryName, downloadURL, opts.Quiet)
	if err != nil {
		return "", err
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", path)

	return path, nil
}

// releaseAssetURL returns the download URL of a k0s release asset.
func (k *K0s) releaseAssetURL(version, assetName string) string {
	return fmt.Sprintf("%s/%s/%s", GitHubReleaseURL, version, assetName)
}
