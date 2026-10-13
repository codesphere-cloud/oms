// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	intutil "github.com/codesphere-cloud/oms/internal/util"
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

// k0sctlConfigGenerator generates a k0sctl config and writes it to disk.
type k0sctlConfigGenerator struct {
	Opts       K0sctlConfigOpts
	Verbose    bool
	Env        env.Env
	FileWriter intutil.FileIO
}

// Generate writes the k0sctl config to outputPath, or to the OMS workdir when outputPath
// is empty, and returns its path.
func (g *k0sctlConfigGenerator) Generate(pm installer.PackageManager, k0s installer.K0sManager, outputPath string) (string, error) {
	if g.Opts.InstallConfig == "" {
		return "", fmt.Errorf("--install-config is required")
	}

	if g.Opts.NoDownload && g.Opts.Airgap {
		return "", fmt.Errorf("--no-download cannot be combined with --airgapped, as the nodes would download k0s from the internet")
	}

	config, err := g.loadInstallConfig()
	if err != nil {
		return "", err
	}

	k0sVersion, err := resolveK0sVersion(k0s, g.Opts.Version)
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

	k0sctlOptions := installer.K0sctlOptions{
		K0sVersion:       k0sVersion,
		SSHKeyPath:       g.Opts.SSHKeyPath,
		K0sBinaryPath:    k0sBinaryPath,
		AirgapBundlePath: airgapBundlePath,
	}

	if outputPath == "" {
		outputPath = filepath.Join(g.Env.GetOmsWorkdir(), fmt.Sprintf("k0sctl-config-%s.yaml", config.Datacenter.Name))
	}

	return g.writeK0sctlConfig(config, k0sctlOptions, outputPath)
}

func (g *k0sctlConfigGenerator) loadInstallConfig() (*files.RootConfig, error) {
	config, err := installer.NewConfig().ParseConfigYaml(g.Opts.InstallConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load install-config: %w", err)
	}

	if !config.Kubernetes.ManagedByCodesphere {
		return nil, fmt.Errorf("install-config specifies external Kubernetes, k0s installation is only supported for Codesphere-managed Kubernetes")
	}

	return &config, nil
}

func (g *k0sctlConfigGenerator) getK0sBinaryPath(pm installer.PackageManager, k0s installer.K0sManager, k0sVersion string) (string, error) {
	if g.Opts.NoDownload {
		return "", nil
	}

	if g.Opts.Package != "" {
		if err := pm.ExtractDependency(defaultK0sPath, g.Opts.Force, g.Verbose); err != nil {
			return "", fmt.Errorf("failed to extract k0s from package: %w", err)
		}

		return pm.GetDependencyPath(defaultK0sPath), nil
	}

	k0sBinaryPath, err := k0s.Download(k0sVersion, installer.DownloadOptions{Force: g.Opts.Force})
	if err != nil {
		return "", fmt.Errorf("failed to download k0s: %w", err)
	}

	return k0sBinaryPath, nil
}

// getAirgapBundlePath returns the local airgap image bundle that k0sctl uploads to the
// worker nodes. It is empty for installations with internet access.
func (g *k0sctlConfigGenerator) getAirgapBundlePath(k0s installer.K0sManager, k0sVersion string) (string, error) {
	if !g.Opts.Airgap {
		return "", nil
	}

	if g.Opts.AirgapBundlePath != "" {
		if !g.FileWriter.Exists(g.Opts.AirgapBundlePath) {
			return "", fmt.Errorf("airgap bundle '%s' does not exist", g.Opts.AirgapBundlePath)
		}

		return g.Opts.AirgapBundlePath, nil
	}

	bundlePath, err := k0s.DownloadAirgapBundle(k0sVersion, installer.DownloadOptions{Force: g.Opts.Force})
	if err != nil {
		return "", fmt.Errorf("failed to download k0s airgap bundle: %w", err)
	}

	return bundlePath, nil
}

func (g *k0sctlConfigGenerator) writeK0sctlConfig(config *files.RootConfig, options installer.K0sctlOptions, path string) (string, error) {
	log.Println("Generating k0sctl configuration from install-config...")

	k0sctlConfig, err := installer.GenerateK0sctlConfig(config, options)
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
