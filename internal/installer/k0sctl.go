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
	k0sctlBinaryName    = "k0sctl"
	k0sctlReleaseURL    = "https://github.com/k0sproject/k0sctl/releases/download"
	k0sctlReleaseAPIURL = "https://api.github.com/repos/k0sproject/k0sctl/releases/latest"
)

// DefaultK0sctlVersion is the currently verified k0sctl version. It mirrors
// DefaultK0sVersion in k0s.go: the pair is the version combination we test
// against, while users can override k0sctl via --k0sctl-version.
//
// renovate: datasource=github-releases depName=k0sproject/k0sctl
const DefaultK0sctlVersion = "v0.33.1"

//mockery:generate: true
type K0sctlManager interface {
	GetLatestVersion() (string, error)
	Download(version string, force bool, quiet bool) (string, error)
	Apply(configPath string, k0sctlPath string, force bool) error
	Reset(configPath string, k0sctlPath string) error
	GetKubeconfig(configPath string, k0sctlPath string) (string, error)
}

type K0sctl struct {
	Env        env.Env
	Http       portal.Http
	FileWriter util.FileIO
	Goos       string
	Goarch     string
}

func NewK0sctl(hw portal.Http, env env.Env, fw util.FileIO) *K0sctl {
	return &K0sctl{
		Env:        env,
		Http:       hw,
		FileWriter: fw,
		Goos:       runtime.GOOS,
		Goarch:     runtime.GOARCH,
	}
}

func (k *K0sctl) GetLatestVersion() (string, error) {
	responseBody, err := k.Http.Get(k0sctlReleaseAPIURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest k0sctl release: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(responseBody, &release); err != nil {
		return "", fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("no tag_name found in GitHub API response")
	}

	return release.TagName, nil
}

func (k *K0sctl) Download(version string, force bool, quiet bool) (string, error) {
	cacheDir, err := ensureCacheDir(k.FileWriter, k.Env)
	if err != nil {
		return "", err
	}

	if version == "" {
		latestVersion, err := k.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest version: %w", err)
		}

		version = latestVersion
		io.Verbosef(!quiet, "Using latest k0sctl version: %s", version)
	}

	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	cachePath := filepath.Join(cacheDir, k0sctlBinaryName)
	if cachedPath, cached := reuseCachedBinary(k.FileWriter, cachePath, version, k0sctlBinaryName, force, quiet); cached {
		return cachedPath, nil
	}

	assetName := fmt.Sprintf("%s-%s-%s", k0sctlBinaryName, k.Goos, k.Goarch)
	downloadURL := releaseAssetURL(k0sctlReleaseURL, version, assetName)

	io.Verbosef(!quiet, "Downloading k0sctl %s from %s", version, downloadURL)

	path, err := downloadBinaryToPath(k.FileWriter, k.Http, cachePath, k0sctlBinaryName, downloadURL, quiet)
	if err != nil {
		return "", err
	}

	io.Verbosef(!quiet, "k0sctl downloaded successfully to %s", path)

	return path, nil
}

// requireBinaryAndConfig checks that both the k0sctl binary and config exist.
func (k *K0sctl) requireBinaryAndConfig(configPath, k0sctlPath string) error {
	if !k.FileWriter.Exists(k0sctlPath) {
		return fmt.Errorf("k0sctl binary does not exist at '%s', please download first", k0sctlPath)
	}

	if !k.FileWriter.Exists(configPath) {
		return fmt.Errorf("k0sctl config does not exist at '%s'", configPath)
	}

	return nil
}

func (k *K0sctl) Apply(configPath string, k0sctlPath string, force bool) error {
	if err := k.requireBinaryAndConfig(configPath, k0sctlPath); err != nil {
		return err
	}

	args := []string{"apply", "--config", configPath}

	if force {
		args = append(args, "--force")
	}

	args = append(args, "--debug")

	log.Printf("Running k0sctl apply with config: %s", configPath)

	err := util.RunCommand(k0sctlPath, args, "")
	if err != nil {
		return fmt.Errorf("k0sctl apply failed: %w", err)
	}

	log.Println("k0sctl apply completed successfully")

	return nil
}

func (k *K0sctl) Reset(configPath string, k0sctlPath string) error {
	if !k.FileWriter.Exists(k0sctlPath) {
		return nil
	}

	if err := k.requireBinaryAndConfig(configPath, k0sctlPath); err != nil {
		return err
	}

	log.Println("Resetting k0s cluster using k0sctl...")

	args := []string{"reset", "--config", configPath, "--force"}

	err := util.RunCommand(k0sctlPath, args, "")
	if err != nil {
		return fmt.Errorf("k0sctl reset failed: %w", err)
	}

	log.Println("k0sctl reset completed successfully")

	return nil
}

func (k *K0sctl) GetKubeconfig(configPath string, k0sctlPath string) (string, error) {
	if err := k.requireBinaryAndConfig(configPath, k0sctlPath); err != nil {
		return "", err
	}

	args := []string{"kubeconfig", "--config", configPath}

	log.Println("Retrieving kubeconfig from k0sctl...")

	output, err := util.RunCommandWithOutput(k0sctlPath, args, "")
	if err != nil {
		return "", fmt.Errorf("k0sctl kubeconfig failed: %w", err)
	}

	return output, nil
}
