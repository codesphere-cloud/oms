// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallerAPI is implemented by the concrete ArgoCD chart installer.
type InstallerAPI interface {
	Install() error
}

// AppInstallerConfig configures the shared ArgoCD, vault secret, and
// pc-applications installation workflow.
type AppInstallerConfig struct {
	Config       files.RootConfig
	Vault        *files.InstallVault
	RESTConfig   *rest.Config
	KubeClient   client.Client
	Installer    InstallerAPI
	PCAppsValues []string
}

// AppInstaller installs ArgoCD, syncs the vault secret, and installs
// pc-applications from the installer BOM.
type AppInstaller struct {
	cfg AppInstallerConfig
}

// NewAppInstaller creates an ArgoCD-and-applications installer.
func NewAppInstaller(cfg AppInstallerConfig) *AppInstaller {
	return &AppInstaller{cfg: cfg}
}

// InstallArgoCD installs or upgrades ArgoCD using the configured installer.
func (i *AppInstaller) InstallArgoCD() error {
	if i.cfg.Installer == nil {
		return fmt.Errorf("ArgoCD installer is required")
	}
	if err := i.cfg.Installer.Install(); err != nil {
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}
	return nil
}

// SyncVaultSecret refreshes the service account tokens and creates or updates
// the Codesphere vault secret in Kubernetes.
func (i *AppInstaller) SyncVaultSecret(ctx context.Context) error {
	if err := secrets.EnsureServiceAccountTokens(i.cfg.Vault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}
	creator := vault.NewVaultSecretCreator(i.cfg.KubeClient)
	if err := creator.CreateSecretFromVault(ctx, i.cfg.Vault, vault.VaultSecretNamespace, vault.VaultSecretName); err != nil {
		return fmt.Errorf("failed to sync vault secret: %w", err)
	}
	return nil
}

// InstallPCApps installs or upgrades pc-applications using the version from
// the supplied installer BOM.
func (i *AppInstaller) InstallPCApps(ctx context.Context, bomPath string) error {
	pcApps, err := installer.NewPcAppsFromBom(
		i.cfg.KubeClient,
		i.cfg.RESTConfig,
		bomPath,
		DefaultNamespace,
		i.cfg.PCAppsValues,
		i.cfg.Config.PcApps,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}
	if err := pcApps.Install(ctx); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
	}
	return nil
}
