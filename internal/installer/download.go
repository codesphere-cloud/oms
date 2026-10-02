// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// DownloadOptions configures how oms fetches an artifact from a remote release
// into the OMS cache.
type DownloadOptions struct {
	// Force downloads the artifact even when a matching cached copy exists.
	Force bool
	// Quiet suppresses progress output.
	Quiet bool
}

// githubReleaseAsset is a single downloadable asset of a GitHub release.
type githubReleaseAsset struct {
	Name string `json:"name"`
}

// githubRelease is the subset of the GitHub release API response that oms uses.
type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

// getGitHubRelease fetches the GitHub release at url and decodes it. subject names
// the release in error messages.
func getGitHubRelease(h portal.Http, url, subject string) (*githubRelease, error) {
	responseBody, err := h.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", subject, err)
	}

	var release githubRelease
	if err := json.Unmarshal(responseBody, &release); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", subject, err)
	}

	return &release, nil
}

func ensureCacheDir(fw util.FileIO, environment env.Env) (string, error) {
	cacheDir, err := environment.GetOmsCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine cache directory: %w", err)
	}

	if err := fw.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workdir: %w", err)
	}

	return cacheDir, nil
}

// reuseCachedBinary returns the cached binary at cachePath and true when it exists,
// matches requestedVersion and opts do not force a fresh download.
func reuseCachedBinary(fw util.FileIO, cachePath, requestedVersion, name string, opts DownloadOptions) (string, bool) {
	if !fw.Exists(cachePath) || opts.Force {
		return "", false
	}

	cachedVersion, versionErr := localBinaryVersion(cachePath)
	if versionErr == nil && cachedVersion == requestedVersion {
		io.Verbosef(!opts.Quiet, "Using cached %s %s at %s", name, requestedVersion, cachePath)

		return cachePath, true
	}

	if versionErr != nil {
		io.Verbosef(!opts.Quiet, "Replacing cached %s: version could not be determined: %v", name, versionErr)
	} else {
		io.Verbosef(!opts.Quiet, "Replacing cached %s %s: requested version %s", name, cachedVersion, requestedVersion)
	}

	return "", false
}

func releaseAssetURL(releaseURL, version, assetName string) string {
	return fmt.Sprintf("%s/%s/%s", releaseURL, version, assetName)
}

func downloadToPath(fw util.FileIO, http portal.Http, path, downloadURL string, quiet bool) error {
	dstFile, err := fw.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %s: %w", path, err)
	}
	defer util.CloseFileIgnoreError(dstFile)

	if err := http.Download(downloadURL, dstFile, quiet); err != nil {
		_ = fw.Remove(path)

		return fmt.Errorf("failed to download %s: %w", path, err)
	}

	return nil
}

func downloadBinaryToPath(fw util.FileIO, http portal.Http, binaryPath, binaryName, downloadURL string, quiet bool) error {
	if err := downloadToPath(fw, http, binaryPath, downloadURL, quiet); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	if err := fw.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make %s binary executable: %w", binaryName, err)
	}

	return nil
}

func localBinaryVersion(binaryPath string) (string, error) {
	output, err := util.RunCommandWithOutput(binaryPath, []string{"version"}, "")
	if err != nil {
		return "", fmt.Errorf("failed to get version of application %s: %w", binaryPath, err)
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if version, found := strings.CutPrefix(line, "version:"); found {
			return strings.TrimSpace(version), nil
		}

		return line, nil
	}

	return "", fmt.Errorf("version output is empty")
}
