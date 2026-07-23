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

//mockery:generate: true
type K0sManager interface {
	GetLatestVersion() (string, error)
	Download(version string, force bool, quiet bool) (string, error)
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
func (k *K0s) Download(version string, force bool, quiet bool) (string, error) {
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

	// The cache uses a stable filename because k0sctl expects a local binary
	// path. Verify that binary before deciding whether it needs replacing.
	// This keeps repeated installs idempotent without accidentally reusing a
	// different k0s release.
	cachePath := filepath.Join(cacheDir, "k0s")
	if k.FileWriter.Exists(cachePath) && !force {
		cachedVersion, versionErr := util.RunCommandWithOutput(cachePath, []string{"version"}, "")
		if versionErr == nil && strings.TrimSpace(cachedVersion) == version {
			if !quiet {
				log.Printf("Using cached k0s %s at %s", version, cachePath)
			}
			return cachePath, nil
		}

		if !quiet {
			if versionErr != nil {
				log.Printf("Cached k0s version could not be determined; replacing it: %v", versionErr)
			} else {
				log.Printf("Cached k0s version %s does not match requested version %s; replacing it", strings.TrimSpace(cachedVersion), version)
			}
		}
	}

	downloadURL := fmt.Sprintf("https://github.com/k0sproject/k0s/releases/download/%s/k0s-%s-%s", version, version, k.Goarch)
	path, err := downloadBinaryToPath(k.FileWriter, k.Http, cachePath, "k0s", downloadURL, quiet)
	if err != nil {
		return "", err
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", path)

	return path, nil
}
