// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/cli/cmd/codesphere"
	cmdutil "github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/spf13/cobra"
)

// RunInstallCodesphereCommand runs the dependency and platform phases after
// preparing the local cluster and its vault.
func (b *LocalBootstrapper) RunInstallCodesphereCommand() (err error) {
	if b.Env.InstallVersion == "" && b.Env.InstallLocal == "" {
		return nil
	}
	if b.installerBundleDir == "" {
		return fmt.Errorf("installer bundle is not prepared")
	}
	dbHost, dbPort, cleanup, err := b.createTemporaryPostgresNodePortEndpoint()
	if err != nil {
		return err
	}
	defer cleanup()
	restore, err := b.configurePostgresForMigration(dbHost, dbPort)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, restore()) }()

	configPath, err := filepath.Abs(b.Env.InstallConfigPath)
	if err != nil {
		return err
	}
	vaultPath, err := filepath.Abs(filepath.Join(b.Env.InstallConfig.Secrets.BaseDir, "prod.vault.yaml"))
	if err != nil {
		return err
	}
	bundleDir, err := filepath.Abs(b.installerBundleDir)
	if err != nil {
		return err
	}
	if b.ageKeyPath == "" {
		return fmt.Errorf("age key path is not set; cannot pass private key to installer")
	}
	privKeyPath, err := filepath.Abs(b.ageKeyPath)
	if err != nil {
		return err
	}
	configDir, err := filepath.Abs(filepath.Join(b.Env.InstallDir, "config"))
	if err != nil {
		return err
	}
	opts := b.localInstallCodesphereOptions(bundleDir, configPath, vaultPath, privKeyPath, configDir)
	command := &cobra.Command{}
	command.SetContext(b.ctx)
	commandEnv := env.NewEnv()
	dependencies := &codesphere.InstallCodesphereDepenciesCmd{Opts: opts, Env: commandEnv}
	if err := dependencies.RunE(command, nil); err != nil {
		return fmt.Errorf("oms install codesphere dependencies failed: %w", err)
	}
	platform := &codesphere.InstallCodespherePlatformCmd{Opts: opts, Env: commandEnv}
	if err := platform.RunE(command, nil); err != nil {
		return fmt.Errorf("oms install codesphere platform failed: %w", err)
	}
	return nil
}

func (b *LocalBootstrapper) localInstallCodesphereOptions(bundleDir, configPath, vaultPath, privKeyPath, configDir string) *codesphere.InstallCodesphereOpts {
	return &codesphere.InstallCodesphereOpts{
		GlobalOptions:        &cmdutil.GlobalOptions{Verbose: b.Verbose},
		Package:              bundleDir,
		Configs:              []string{configPath},
		Vault:                vaultPath,
		PrivKey:              privKeyPath,
		VaultType:            string(vault.TypeSOPS),
		LocalComponents:      true,
		LocalConfigDir:       configDir,
		AutoApprove:          true,
		ArgoCDVersion:        "9.5.21",
		ArgoCDForceConflicts: true,
		ArgoCDRepoURL:        argocd.DefaultRepoURL,
		ArgoCDRegistryURL:    strings.TrimPrefix(b.Env.ArgoCDRegistryURL, "oci://"),
	}
}
