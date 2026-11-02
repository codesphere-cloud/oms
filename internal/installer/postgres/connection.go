// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"gopkg.in/yaml.v3"
)

// NewInstaller loads the vault and Kubernetes client shared by the installation phases.
func NewInstaller(ctx context.Context, vaultPath, privKey, vaultType string, cfg files.RootConfig) (*Installer, *files.InstallVault, error) {
	installVault, restConfig, err := installer.VaultAndRESTConfig(vaultPath, privKey, vaultType, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("load masterdata database vault and Kubernetes config: %w", err)
	}

	kube, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create masterdata database Kubernetes client: %w", err)
	}

	pg := &Installer{Context: ctx, KubeClient: kube, Config: cfg.Postgres.K8sManagedConfig}

	return pg, installVault, nil
}

// WriteConnectionConfig reads the existing CA and writes legacy-compatible
// PostgreSQL settings to the temporary installation config.
func (b *Installer) WriteConnectionConfig(configPath string, cfg *files.RootConfig) error {
	ca, err := b.ReadCA()
	if err != nil {
		return fmt.Errorf("read masterdata database CA: %w", err)
	}

	cfg.Postgres = ConnectionConfig(ca)
	cfg.Cluster.PgOperator = &files.PgOperatorConfig{Enabled: false, ManagedExternally: true}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read merged installation config: %w", err)
	}

	var merged map[string]any
	if err := yaml.Unmarshal(data, &merged); err != nil {
		return fmt.Errorf("parse merged installation config: %w", err)
	}

	merged["postgres"] = cfg.Postgres
	if cfg.Codesphere.Migration != nil {
		codesphere, ok := merged["codesphere"].(map[string]any)
		if !ok {
			codesphere = map[string]any{}
			merged["codesphere"] = codesphere
		}

		codesphere["migration"] = cfg.Codesphere.Migration
	}

	cluster, ok := merged["cluster"].(map[string]any)
	if !ok {
		cluster = map[string]any{}
		merged["cluster"] = cluster
	}

	cluster["pgOperator"] = cfg.Cluster.PgOperator

	data, err = yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal masterdata database connection config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("write masterdata database connection config: %w", err)
	}

	return nil
}
