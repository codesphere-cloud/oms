// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

type K0sctlManager interface {
	GetLatestVersion() (string, error)
	Download(version string, force bool, quiet bool) (string, error)
	Apply(configPath string, k0sctlPath string, force bool) error
	Reset(configPath string, k0sctlPath string) error
}

type K0sctl struct {
	Env        env.Env
	Http       portal.Http
	FileWriter util.FileIO
	Goos       string
	Goarch     string
}

func NewK0sctl(hw portal.Http, env env.Env, fw util.FileIO) K0sctlManager {
	return &K0sctl{
		Env:        env,
		Http:       hw,
		FileWriter: fw,
		Goos:       runtime.GOOS,
		Goarch:     runtime.GOARCH,
	}
}

// githubRelease represents the minimal GitHub release API response
type githubRelease struct {
	TagName string `json:"tag_name"`
}

func (k *K0sctl) GetLatestVersion() (string, error) {
	// k0sctl uses GitHub releases - fetch latest from API
	releaseURL := "https://api.github.com/repos/k0sproject/k0sctl/releases/latest"
	responseBody, err := k.Http.Get(releaseURL)
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
	if version == "" {
		var err error
		version, err = k.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest version: %w", err)
		}
		if !quiet {
			log.Printf("Using latest k0sctl version: %s", version)
		}
	}

	// Ensure workdir exists
	workdir := k.Env.GetOmsWorkdir()
	if err := os.MkdirAll(workdir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workdir: %w", err)
	}

	k0sctlPath := filepath.Join(workdir, "k0sctl")
	if k.FileWriter.Exists(k0sctlPath) && !force {
		return "", fmt.Errorf("k0sctl binary already exists at %s. Use --force to overwrite", k0sctlPath)
	}

	// Construct download URL
	// Format: https://github.com/k0sproject/k0sctl/releases/download/v0.17.4/k0sctl-linux-amd64
	binaryName := fmt.Sprintf("k0sctl-%s-%s", k.Goos, k.Goarch)
	// Ensure version has v prefix for GitHub URL
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	downloadURL := fmt.Sprintf("https://github.com/k0sproject/k0sctl/releases/download/%s/%s", version, binaryName)

	if !quiet {
		log.Printf("Downloading k0sctl %s from %s", version, downloadURL)
	}

	dstFile, err := k.FileWriter.Create(k0sctlPath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer util.IgnoreError(dstFile.Close)

	if err := k.Http.Download(downloadURL, dstFile, quiet); err != nil {
		return "", fmt.Errorf("failed to download k0sctl: %w", err)
	}

	// Make binary executable
	if err := os.Chmod(k0sctlPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make k0sctl executable: %w", err)
	}

	if !quiet {
		log.Printf("k0sctl downloaded successfully to %s", k0sctlPath)
	}

	return k0sctlPath, nil
}

func (k *K0sctl) Apply(configPath string, k0sctlPath string, force bool) error {
	if !k.FileWriter.Exists(k0sctlPath) {
		return fmt.Errorf("k0sctl binary does not exist at '%s', please download first", k0sctlPath)
	}

	if !k.FileWriter.Exists(configPath) {
		return fmt.Errorf("k0sctl config does not exist at '%s'", configPath)
	}

	args := []string{"apply", "--config", configPath}

	if force {
		args = append(args, "--force")
	}

	// Add debug flag for more verbose output
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

	if !k.FileWriter.Exists(configPath) {
		return fmt.Errorf("k0sctl config does not exist at '%s'", configPath)
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
