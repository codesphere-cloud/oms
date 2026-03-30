// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
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

// Download downloads the k0s binary for the specified version and saves it to the OMS workdir.
func (k *K0s) Download(version string, force bool, quiet bool) (string, error) {
	if k.Goos != "linux" || k.Goarch != "amd64" {
		return "", fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", k.Goos, k.Goarch)
	}

	log.Printf("Downloading k0s version %s", version)

	downloadURL := fmt.Sprintf("https://github.com/k0sproject/k0s/releases/download/%s/k0s-%s-%s", version, version, k.Goarch)
	path, err := downloadBinary(k.FileWriter, k.Http, k.Env.GetOmsWorkdir(), "k0s", downloadURL, force, quiet)
	if err != nil {
		return "", err
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", path)

	return path, nil
}
