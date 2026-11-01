// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/postgres"
)

func (b *LocalBootstrapper) postgresInstaller() *postgres.Installer {
	return &postgres.Installer{
		Context: b.ctx, KubeClient: b.kubeClient, HelmClient: b.helmClient,
		BOM: b.installerBOM, RegistryUser: b.Env.RegistryUser,
		RegistryPassword: b.Env.RegistryPassword, StorageClass: b.storageClassName(),
	}
}

// InstallCloudNativePGHelmChart installs the operator from the installer BOM.
func (b *LocalBootstrapper) InstallCloudNativePGHelmChart() error {
	repoURL, err := b.Env.chartsRegistryURL()
	if err != nil {
		return err
	}

	pg := b.postgresInstaller()
	pg.RegistryURL = repoURL

	if err := pg.InstallOperator(); err != nil {
		return fmt.Errorf("install CloudNativePG operator: %w", err)
	}
	return nil
}

// DeployPostgresDatabase creates the local bootstrap database.
func (b *LocalBootstrapper) DeployPostgresDatabase() error {
	if err := b.postgresInstaller().DeployDatabase(); err != nil {
		return fmt.Errorf("deploy PostgreSQL database: %w", err)
	}

	return nil
}

// ReadPostgresSuperuserPassword reads the CNPG generated superuser password.
func (b *LocalBootstrapper) ReadPostgresSuperuserPassword() (string, error) {
	password, err := b.postgresInstaller().ReadSuperuserPassword()
	if err != nil {
		return "", fmt.Errorf("read PostgreSQL superuser password: %w", err)
	}

	return password, nil
}

// ReadPostgresCA reads the CNPG generated CA certificate.
func (b *LocalBootstrapper) ReadPostgresCA() (string, error) {
	ca, err := b.postgresInstaller().ReadCA()
	if err != nil {
		return "", fmt.Errorf("read PostgreSQL CA: %w", err)
	}

	return ca, nil
}
