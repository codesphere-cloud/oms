// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"gopkg.in/yaml.v3"
)

type K0sManager interface {
	GetLatestVersion() (string, error)
	Download(version string, force bool, quiet bool) (string, error)
	Install(configPath string, k0sPath string, force bool) error
	Reset(k0sPath string) error
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

	// Check if k0s binary already exists and create destination file
	workdir := k.Env.GetOmsWorkdir()

	// Ensure workdir exists
	if err := os.MkdirAll(workdir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workdir: %w", err)
	}

	k0sPath := filepath.Join(workdir, "k0s")
	if k.FileWriter.Exists(k0sPath) && !force {
		return "", fmt.Errorf("k0s binary already exists at %s. Use --force to overwrite", k0sPath)
	}

	file, err := k.FileWriter.Create(k0sPath)
	if err != nil {
		return "", fmt.Errorf("failed to create k0s binary file: %w", err)
	}
	defer util.CloseFileIgnoreError(file)

	// Download using the portal Http wrapper with WriteCounter
	log.Printf("Downloading k0s version %s", version)

	downloadURL := fmt.Sprintf("https://github.com/k0sproject/k0s/releases/download/%s/k0s-%s-%s", version, version, k.Goarch)
	err = k.Http.Download(downloadURL, file, quiet)
	if err != nil {
		return "", fmt.Errorf("failed to download k0s binary: %w", err)
	}

	// Make the binary executable
	err = os.Chmod(k0sPath, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to make k0s binary executable: %w", err)
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", k0sPath)

	return k0sPath, nil
}

func (k *K0s) Install(configPath string, k0sPath string, force bool) error {
	if k.Goos != "linux" || k.Goarch != "amd64" {
		return fmt.Errorf("k0s installation is only supported on Linux amd64. Current platform: %s/%s", k.Goos, k.Goarch)
	}

	if !k.FileWriter.Exists(k0sPath) {
		return fmt.Errorf("k0s binary does not exist in '%s', please download first", k0sPath)
	}

	if force {
		if err := k.Reset(k0sPath); err != nil {
			log.Printf("Warning: failed to reset k0s: %v", err)
		}
	}

	args := []string{k0sPath, "install", "controller"}

	// If config path is provided, filter it to only include k0s-compatible fields
	if configPath != "" {
		filteredConfigPath, err := k.filterConfigForK0s(configPath)
		if err != nil {
			log.Printf("Warning: failed to filter config, using original: %v", err)
		} else {
			configPath = filteredConfigPath
			defer os.Remove(filteredConfigPath) // Clean up temp file after use
		}
		args = append(args, "--config", configPath)
	} else {
		args = append(args, "--single")
	}

	if force {
		args = append(args, "--force")
	}

	err := util.RunCommand("sudo", args, "")
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	if configPath != "" {
		log.Println("k0s installed successfully with provided configuration.")
	} else {
		log.Println("k0s installed successfully in single-node mode.")
	}
	log.Printf("You can start it using 'sudo %v start'", k0sPath)
	log.Printf("You can check the status using 'sudo %v status'", k0sPath)

	return nil
}

func (k *K0s) filterConfigForK0s(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse config: %w", err)
	}

	keysToKeep := map[string]bool{
		"apiVersion": true,
		"kind":       true,
		"metadata":   true,
		"spec":       true,
	}

	for key := range config {
		if !keysToKeep[key] {
			delete(config, key)
		}
	}

	if spec, ok := config["spec"].(map[string]interface{}); ok {
		specKeysToKeep := map[string]bool{
			"api":               true,
			"controllerManager": true,
			"scheduler":         true,
			"extensions":        true,
			"network":           true,
			"storage":           true,
			"telemetry":         true,
			"images":            true,
			"konnectivity":      true,
		}

		for key := range spec {
			if !specKeysToKeep[key] {
				delete(spec, key)
			}
		}
		config["spec"] = spec
	}

	filteredData, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal filtered config: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "k0s-config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp config: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(filteredData); err != nil {
		return "", fmt.Errorf("failed to write temp config: %w", err)
	}

	return tmpFile.Name(), nil
}

func (k *K0s) Reset(k0sPath string) error {
	if !k.FileWriter.Exists(k0sPath) {
		return nil
	}

	log.Println("Resetting existing k0s installation...")
	err := util.RunCommand("sudo", []string{k0sPath, "reset"}, "")
	if err != nil {
		return fmt.Errorf("failed to reset k0s: %w", err)
	}

	log.Println("k0s reset completed successfully")
	return nil
}
