// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ArgoCDNamespace is the Kubernetes namespace used for ArgoCD and the
// pc-applications installation.
const ArgoCDNamespace = "argocd"

// ArgoCDInstaller is implemented by the concrete ArgoCD chart installer.
type ArgoCDInstaller interface {
	Install() error
}

// ArgoCDAndAppsInstallConfig configures the shared ArgoCD, vault secret, and
// pc-applications installation workflow.
type ArgoCDAndAppsInstallConfig struct {
	Context context.Context

	Config          files.RootConfig
	Vault           *files.InstallVault
	RESTConfig      *rest.Config
	KubeClient      client.Client
	ArgoCDInstaller ArgoCDInstaller
	PCAppsValues    []string
}

// ArgoCDAndAppsInstall installs ArgoCD, syncs the vault secret, and installs
// pc-applications from the installer BOM.
type ArgoCDAndAppsInstall struct {
	cfg ArgoCDAndAppsInstallConfig
}

// NewArgoCDAndAppsInstall creates an ArgoCD-and-applications installer.
func NewArgoCDAndAppsInstall(cfg ArgoCDAndAppsInstallConfig) *ArgoCDAndAppsInstall {
	if cfg.Context == nil {
		cfg.Context = context.Background()
	}
	return &ArgoCDAndAppsInstall{cfg: cfg}
}

// InstallArgoCD installs or upgrades ArgoCD using the configured installer.
func (i *ArgoCDAndAppsInstall) InstallArgoCD() error {
	if i.cfg.ArgoCDInstaller == nil {
		return fmt.Errorf("ArgoCD installer is required")
	}
	if err := i.cfg.ArgoCDInstaller.Install(); err != nil {
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}
	return nil
}

// SyncVaultSecret refreshes the service account tokens and creates or updates
// the Codesphere vault secret in Kubernetes.
func (i *ArgoCDAndAppsInstall) SyncVaultSecret() error {
	if err := secrets.EnsureServiceAccountTokens(i.cfg.Vault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}
	creator := vault.NewVaultSecretCreator(i.cfg.KubeClient)
	if err := creator.CreateSecretFromVault(i.cfg.Context, i.cfg.Vault, vault.VaultSecretNamespace, vault.VaultSecretName); err != nil {
		return fmt.Errorf("failed to sync vault secret: %w", err)
	}
	return nil
}

// InstallPCApps installs or upgrades pc-applications using the version from
// the supplied installer BOM.
func (i *ArgoCDAndAppsInstall) InstallPCApps(bomPath string) error {
	pcApps, err := NewPcAppsFromBom(
		i.cfg.KubeClient,
		i.cfg.RESTConfig,
		bomPath,
		ArgoCDNamespace,
		i.cfg.PCAppsValues,
		i.cfg.Config.PcApps,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}
	if err := pcApps.Install(i.cfg.Context); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
	}
	return nil
}
