// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
)

// MasterdataDBOptions supplies installation artifacts and credential locations.
type MasterdataDBOptions struct {
	ConfigPath  string
	VaultPath   string
	PrivKey     string
	VaultType   string
	BOM         *bom.Config
	RegistryURL string
}

// InstallMasterdataDB deploys PostgreSQL and persists its credentials during dependencies.
func InstallMasterdataDB(ctx context.Context, cfg *files.RootConfig, opts MasterdataDBOptions) error {
	if cfg.Postgres.Mode != "k8s-managed" {
		return nil
	}

	pg, installVault, err := NewInstaller(ctx, opts.VaultPath, opts.PrivKey, opts.VaultType, *cfg)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL installer: %w", err)
	}

	if _, err := pg.ClusterSpec(); err != nil {
		return fmt.Errorf("validate masterdata database deployment: %w", err)
	}

	if err := pg.EnsureImagePullSecret(ctx); err != nil {
		return fmt.Errorf("provision masterdata image pull credentials: %w", err)
	}

	operatorValues, err := OperatorValues(*cfg, opts.BOM)
	if err != nil {
		return fmt.Errorf("configure CloudNativePG chart: %w", err)
	}

	// PgOperator flags control the legacy installer; Argo CD owns this operator.
	if err := argocdinstaller.InstallCNPGOperator(ctx, pg.KubeClient, opts.BOM, opts.RegistryURL, operatorValues); err != nil {
		return fmt.Errorf("install CloudNativePG: %w", err)
	}

	if err := pg.DeployDatabase(); err != nil {
		return fmt.Errorf("deploy masterdata database: %w", err)
	}

	password, err := pg.ReadSuperuserPassword()
	if err != nil {
		return fmt.Errorf("read masterdata database password: %w", err)
	}

	installVault.SetSecret(files.SecretEntry{Name: files.SecretPostgresPassword, Fields: &files.SecretFields{Password: password}})

	vaultPath, err := installer.ResolveVaultPath(opts.VaultPath, *cfg)
	if err != nil {
		return fmt.Errorf("resolve masterdata database vault path: %w", err)
	}

	store, err := vault.NewFromString(opts.VaultType, vault.Options{Path: vaultPath, AgeKey: opts.PrivKey})
	if err != nil {
		return fmt.Errorf("open masterdata database vault: %w", err)
	}

	if err := store.Save(installVault); err != nil {
		return fmt.Errorf("save masterdata database credentials: %w", err)
	}

	if err := pg.WriteConnectionConfig(opts.ConfigPath, cfg); err != nil {
		return fmt.Errorf("configure PostgreSQL connection: %w", err)
	}

	return nil
}
