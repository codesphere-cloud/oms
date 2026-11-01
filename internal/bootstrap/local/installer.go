// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/portal"
)

// installerArtifactFilename is the artifact to download from the OMS portal.
const installerArtifactFilename = "installer-lite.tar.gz"

// DownloadInstallerPackage downloads the Codesphere installer package from the
// OMS portal, similar to how the GCP bootstrapper fetches it onto a jumpbox.
// The package is downloaded into the directory that contains the config/secrets
// files and its on-disk filename is returned.
func (b *LocalBootstrapper) DownloadInstallerPackage() (string, error) {
	if b.Env.InstallConfig == nil {
		return "", fmt.Errorf("install config must be prepared before downloading a package")
	}

	version, err := b.Env.InstallConfig.CodespherePackageVersion()
	if err != nil {
		return "", fmt.Errorf("resolve installer package version: %w", err)
	}

	filename, err := portal.EnsureCodespherePackage(portal.NewPortalClient(), b.Env.InstallDir, version, b.Env.InstallConfig.Operations.Codesphere.Hash, installerArtifactFilename, false)
	if err != nil {
		return "", fmt.Errorf("prepare installer package: %w", err)
	}

	return filename, nil
}

// PrepareInstallerBundle resolves the installer package to a directory.
// It handles three cases:
//  1. Portal download: InstallVersion is set → download tar.gz, then extract.
//  2. Local tar.gz/tgz: InstallLocal points to an archive → extract.
//  3. Local directory: InstallLocal points to an already-unpacked directory → use as-is.
func (b *LocalBootstrapper) PrepareInstallerBundle() (string, error) {
	var bundlePath string

	switch {
	case b.Env.InstallVersion != "":
		// Download from portal.
		downloaded, err := b.DownloadInstallerPackage()
		if err != nil {
			return "", err
		}
		bundlePath = downloaded

	case b.Env.InstallLocal != "":
		bundlePath = b.Env.InstallLocal

	default:
		return "", fmt.Errorf("either --install-version or --install-local must be specified")
	}

	info, err := os.Stat(bundlePath)
	if err != nil {
		return "", fmt.Errorf("cannot access installer bundle %q: %w", bundlePath, err)
	}

	// Already an unpacked directory – use directly.
	if info.IsDir() {
		log.Printf("Installer bundle is a directory, using as-is: %s", bundlePath)
		return bundlePath, nil
	}

	// Treat as tar.gz archive – extract alongside the archive.
	if !strings.HasSuffix(bundlePath, ".tar.gz") && !strings.HasSuffix(bundlePath, ".tgz") {
		return "", fmt.Errorf("installer bundle %q is neither a directory nor a .tar.gz/.tgz archive", bundlePath)
	}

	packageManager := installer.NewPackage(filepath.Dir(bundlePath), bundlePath)
	if err := packageManager.Extract(false, b.Verbose); err != nil {
		return "", fmt.Errorf("failed to extract installer bundle: %w", err)
	}

	return packageManager.GetWorkDir(), nil
}

// PrepareInstaller resolves and extracts the installer bundle before the
// bootstrap needs any version information from its BOM.
func (b *LocalBootstrapper) PrepareInstaller() error {
	if b.Env.InstallVersion == "" && b.Env.InstallLocal == "" {
		return nil
	}

	bundleDir, err := b.PrepareInstallerBundle()
	if err != nil {
		return err
	}

	bomConfig, err := bom.Parse(filepath.Join(bundleDir, "deps", "bom.json"))
	if err != nil {
		return fmt.Errorf("failed to parse installer BOM: %w", err)
	}

	b.installerBundleDir = bundleDir
	b.installerBOM = bomConfig

	return nil
}

func (b *LocalBootstrapper) configurePostgresForMigration(host string, port int32) (func() error, error) {
	previousMigration := b.Env.InstallConfig.Codesphere.Migration
	b.Env.InstallConfig.Codesphere.Migration = &files.MigrationConfig{
		Postgres: &files.MigrationPostgresConfig{
			Host:     host,
			Port:     int(port),
			Database: b.Env.InstallConfig.Postgres.Database,
			AltName:  b.Env.InstallConfig.Postgres.ServerAddress,
		},
	}

	if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
		b.Env.InstallConfig.Codesphere.Migration = previousMigration
		return nil, fmt.Errorf("failed to write migration config to install config: %w", err)
	}

	return func() error {
		b.Env.InstallConfig.Codesphere.Migration = previousMigration
		if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
			return fmt.Errorf("failed to restore install config after installer run: %w", err)
		}
		return nil
	}, nil
}
