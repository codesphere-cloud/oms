// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
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

	// BinaryName is the name of target binary for oms to download to
	BinaryName = "k0s"

	// AirgapBundleName is the name of target airgap-bundle for oms to download to
	AirgapBundleName = "k0s-airgap-bundle"
)

//mockery:generate: true
type K0sManager interface {
	GetLatestVersion() (string, error)
	Download(version string, force bool, quiet bool, airgapped bool) (string, error)
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
		err = k.downloadAirgappedBundle(version, cacheDir, force, quiet)
		if err != nil {
			return "", fmt.Errorf("failed to download k0s-airgapped bundle: %w", err)
		}
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", path)

	return path, nil
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

// downloadAirgappedBundle fetches the k0s airgap image bundle for the given version
// from the k0s GitHub releases and stores it as tar in cacheDir
// If a bundle for that version is already cached and force is false, the cached bundle
// is reused; otherwise it is replaced by a fresh download.
func (k *K0s) downloadAirgappedBundle(version, cacheDir string, force, quiet bool) error {
	bundleName := fmt.Sprintf("%s-%s-%s-%s.tar", AirgapBundleName, version, k.Goos, k.Goarch)

	cachePath := filepath.Join(cacheDir, bundleName)
	if k.FileWriter.Exists(cachePath) && !force {
		io.Verbosef(!quiet, "Using cached %s at %s", bundleName, cachePath)
		return nil
	}

	downloadURL := fmt.Sprintf("%s/%s/%s", GitHubReleaseURL, version, bundleName)

	err := downloadToPath(k.FileWriter, k.Http, cachePath, downloadURL, quiet)
	if err != nil {
		return err
	}

	return err
}
