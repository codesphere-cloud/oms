// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

//mockery:generate: true
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

func NewK0sctl(hw portal.Http, env env.Env, fw util.FileIO) *K0sctl {
	return &K0sctl{
		Env:        env,
		Http:       hw,
		FileWriter: fw,
		Goos:       runtime.GOOS,
		Goarch:     runtime.GOARCH,
	}
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func (k *K0sctl) GetLatestVersion() (string, error) {
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

	// Ensure version has v prefix for GitHub URL
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	binaryName := fmt.Sprintf("k0sctl-%s-%s", k.Goos, k.Goarch)
	downloadURL := fmt.Sprintf("https://github.com/k0sproject/k0sctl/releases/download/%s/%s", version, binaryName)

	if !quiet {
		log.Printf("Downloading k0sctl %s from %s", version, downloadURL)
	}

	path, err := downloadBinary(k.FileWriter, k.Http, k.Env.GetOmsWorkdir(), "k0sctl", downloadURL, force, quiet)
	if err != nil {
		return "", err
	}

	if !quiet {
		log.Printf("k0sctl downloaded successfully to %s", path)
	}

	return path, nil
}

func (k *K0sctl) WriteKubeconfig(k0sctlPath, configPath, kubeconfigPath string) error {
	if !k.FileWriter.Exists(k0sctlPath) {
		return fmt.Errorf("k0sctl binary does not exist at '%s', please download first", k0sctlPath)
	}

	if !k.FileWriter.Exists(configPath) {
		return fmt.Errorf("k0sctl config does not exist at '%s'", configPath)
	}

	args := []string{"kubeconfig", "--config", configPath}

	log.Printf("Running k0sctl kubeconfig with config: %s", configPath)

	output, err := util.RunCommandGetOutput(k0sctlPath, args, "")
	if err != nil {
		return fmt.Errorf("k0sctl kubeconfig failed: %w", err)
	}

	err = k.FileWriter.WriteFile(kubeconfigPath, []byte(output), 0644)
	if err != nil {
		return fmt.Errorf("failed to write kubeconfig file: %w", err)
	}
	log.Println("k0sctl kubeconfig completed successfully")
	return nil
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
