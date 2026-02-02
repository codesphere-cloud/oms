// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

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

// GetNodeIPAddress finds the IP address of the current node by matching
// against the control plane IPs in the config. Returns matching control plane IP
// if found, otherwise returns the first non-loopback IPv4 address.
func GetNodeIPAddress(controlPlanes []string) (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Build a set of control plane IPs for O(1) lookup
	cpSet := make(map[string]bool, len(controlPlanes))
	for _, ip := range controlPlanes {
		cpSet[ip] = true
	}

	var fallbackIP string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}

		ip := ipnet.IP.String()
		if cpSet[ip] {
			return ip, nil
		}
		if fallbackIP == "" {
			fallbackIP = ip
		}
	}

	if fallbackIP != "" {
		return fallbackIP, nil
	}

	return "", fmt.Errorf("no suitable IP address found")
}
