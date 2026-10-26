// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const defaultK0sPath = "kubernetes/files/k0s"

// K0sctlConfigOpts are the options from which oms generates a k0sctl config out of a
// Codesphere install-config.
type K0sctlConfigOpts struct {
	Version       string
	Package       string
	InstallConfig string
	SSHKeyPath    string
	Force         bool
	NoDownload    bool
	// Airgap uploads the airgap image bundle to the nodes and stops k0s from pulling images.
	Airgap bool
	// AirgapBundlePath is the airgap image bundle to upload. Empty uses the OMS cache.
	AirgapBundlePath string
}

// K0sctlConfigGenerator generates a k0sctl config and writes it to disk.
type K0sctlConfigGenerator struct {
	Opts       K0sctlConfigOpts
	Verbose    bool
	Env        env.Env
	FileWriter util.FileIO
	// CacheOnly takes the k0s binary and the airgap bundle from the OMS cache and fails
	// if they are missing, instead of downloading them.
	CacheOnly bool
}

// Generate writes the k0sctl config to outputPath, or to the OMS workdir when outputPath
// is empty, and returns its path.
func (g *K0sctlConfigGenerator) Generate(pm PackageManager, k0s K0sManager, outputPath string) (string, error) {
	if g.Opts.InstallConfig == "" {
		return "", fmt.Errorf("--install-config is required")
	}

	if g.Opts.NoDownload && g.Opts.Airgap {
		return "", fmt.Errorf("--no-download cannot be combined with --airgapped, as the nodes would download k0s from the internet")
	}

	config, err := LoadManagedInstallConfig(g.Opts.InstallConfig)
	if err != nil {
		return "", err
	}

	k0sVersion, err := ResolveK0sVersion(k0s, g.Opts.Version)
	if err != nil {
		return "", err
	}

	k0sBinaryPath, err := g.getK0sBinaryPath(pm, k0s, k0sVersion)
	if err != nil {
		return "", err
	}

	airgapBundlePath, err := g.getAirgapBundlePath(k0s, k0sVersion)
	if err != nil {
		return "", err
	}

	sshKeyPath, err := absolutePath(g.Opts.SSHKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve SSH key path: %w", err)
	}

	k0sctlOptions := K0sctlOptions{
		K0sVersion:       k0sVersion,
		SSHKeyPath:       sshKeyPath,
		K0sBinaryPath:    k0sBinaryPath,
		AirgapBundlePath: airgapBundlePath,
	}

	if outputPath == "" {
		outputPath = filepath.Join(g.Env.GetOmsWorkdir(), fmt.Sprintf("k0sctl-config-%s.yaml", config.Datacenter.Name))
	}

	return g.writeK0sctlConfig(config, k0sctlOptions, outputPath)
}

// ResolveK0sVersion returns the requested k0s version, or the latest version when
// none was requested, and logs the version that is used.
func ResolveK0sVersion(k0s K0sManager, version string) (string, error) {
	if version == "" {
		latestVersion, err := k0s.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest k0s version: %w", err)
		}

		version = latestVersion
	}

	log.Printf("Using k0s version %s", version)

	return version, nil
}

// LoadManagedInstallConfig loads the install-config and ensures its Kubernetes is managed by
// Codesphere, as only then OMS handles k0s on its nodes.
func LoadManagedInstallConfig(path string) (*files.RootConfig, error) {
	config, err := NewConfig().ParseConfigYaml(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load install-config: %w", err)
	}

	if !config.Kubernetes.ManagedByCodesphere {
		return nil, fmt.Errorf("install-config specifies external Kubernetes, k0s installation is only supported for Codesphere-managed Kubernetes")
	}

	return &config, nil
}

// downloadOptions returns the options for fetching the k0s binary and the airgap bundle.
// A cache-only lookup never forces a download.
func (g *K0sctlConfigGenerator) downloadOptions() DownloadOptions {
	if g.CacheOnly {
		return DownloadOptions{CacheOnly: true}
	}

	return DownloadOptions{Force: g.Opts.Force}
}

func (g *K0sctlConfigGenerator) getK0sBinaryPath(pm PackageManager, k0s K0sManager, k0sVersion string) (string, error) {
	if g.Opts.NoDownload {
		return "", nil
	}

	if g.Opts.Package != "" {
		if err := pm.ExtractDependency(defaultK0sPath, g.Opts.Force, g.Verbose); err != nil {
			return "", fmt.Errorf("failed to extract k0s from package: %w", err)
		}

		return pm.GetDependencyPath(defaultK0sPath), nil
	}

	k0sBinaryPath, err := k0s.Download(k0sVersion, g.downloadOptions())
	if err != nil {
		return "", fmt.Errorf("failed to get k0s binary: %w", err)
	}

	return k0sBinaryPath, nil
}

// getAirgapBundlePath returns the local airgap image bundle that k0sctl uploads to the
// worker nodes. It is empty for installations with internet access.
func (g *K0sctlConfigGenerator) getAirgapBundlePath(k0s K0sManager, k0sVersion string) (string, error) {
	if !g.Opts.Airgap {
		return "", nil
	}

	if g.Opts.AirgapBundlePath != "" {
		bundlePath, err := absolutePath(g.Opts.AirgapBundlePath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve airgap bundle path: %w", err)
		}

		if !g.FileWriter.Exists(bundlePath) {
			return "", fmt.Errorf("airgap bundle '%s' does not exist", bundlePath)
		}

		return bundlePath, nil
	}

	bundlePath, err := k0s.DownloadAirgapBundle(k0sVersion, "", g.downloadOptions())
	if err != nil {
		return "", fmt.Errorf("failed to get k0s airgap bundle: %w", err)
	}

	return bundlePath, nil
}

// absolutePath makes a path absolute, so that the k0sctl config works independent of the
// directory k0sctl is run from. Empty paths and paths starting with '~', which k0sctl
// expands itself, are returned unchanged.
func absolutePath(path string) (string, error) {
	if path == "" || strings.HasPrefix(path, "~") {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path of '%s': %w", path, err)
	}

	return absPath, nil
}

func (g *K0sctlConfigGenerator) writeK0sctlConfig(config *files.RootConfig, options K0sctlOptions, path string) (string, error) {
	log.Println("Generating k0sctl configuration from install-config...")

	k0sctlConfig, err := GenerateK0sctlConfig(config, options)
	if err != nil {
		return "", fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	k0sctlConfigData, err := k0sctlConfig.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal k0sctl config: %w", err)
	}

	if err := g.FileWriter.WriteFile(path, k0sctlConfigData, 0644); err != nil {
		return "", fmt.Errorf("failed to write k0sctl config: %w", err)
	}

	log.Printf("Generated k0sctl configuration at %s", path)

	return path, nil
}
