// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"
	"log"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Opts holds all parameters for running the ArgoCD integration sequence.
type Opts struct {
	// BomPath is the filesystem path to bom.json.
	BomPath string
	// DatacenterID is the Codesphere datacenter identifier (e.g. "0").
	DatacenterID string
	// OCIPassword is the OCI registry password for the Helm pull secret.
	OCIPassword string
	// RegistryURL is the Helm OCI registry URL (e.g. "ghcr.io/codesphere-cloud/charts").
	RegistryURL string
	// InstallArgoCD controls whether the ArgoCD Helm chart is installed before
	// applying resources. Set to true for fresh clusters; false when ArgoCD is
	// already present (e.g. installed by the private-cloud-installer script).
	InstallArgoCD bool
	// VaultFile is the path to a SOPS-encrypted vault file to deploy as a
	// Kubernetes secret. Skipped when empty.
	VaultFile string
	// AgeKeyPath is the age private key used to decrypt VaultFile.
	AgeKeyPath string
	// VaultNamespace is the Kubernetes namespace for the vault secret.
	VaultNamespace string
	// VaultSecretName is the name of the Kubernetes secret created from VaultFile.
	VaultSecretName string
}

const (
	DefaultVaultNamespace  = "codesphere"
	DefaultVaultSecretName = "cs-vault"
)

// ArgoCDInstaller can install or upgrade the ArgoCD Helm chart.
type ArgoCDInstaller interface {
	Install() error
}

// ArgoCDResourcesApplier applies ArgoCD-managed resources (pull secrets, AppProjects, etc.).
type ArgoCDResourcesApplier interface {
	ApplyAll(ctx context.Context) error
}

// VaultSecretsDeployer deploys a SOPS-encrypted vault file as a Kubernetes secret.
type VaultSecretsDeployer interface {
	CreateSecretFromVault(ctx context.Context, vaultFile, ageKeyPath, namespace, secretName string) error
}

// PCAppsRunner installs the pc-applications Helm chart.
type PCAppsRunner interface {
	Install(ctx context.Context) error
}

// Deps holds factory functions for the ArgoCD integration dependencies.
// Replace individual factories in tests to inject mocks.
type Deps struct {
	NewArgoCDInstaller      func(dcId, ociPw, registryURL string) (ArgoCDInstaller, error)
	NewArgoCDResources      func(dcId, ociPw, registryURL string) (ArgoCDResourcesApplier, error)
	NewVaultSecretsDeployer func(ctrlclient.Client) VaultSecretsDeployer
	NewPCAppsRunner         func(ctrlclient.Client, string, string) (PCAppsRunner, error)
}

func defaultDeps() Deps {
	return Deps{
		NewArgoCDInstaller: func(dcId, ociPw, registryURL string) (ArgoCDInstaller, error) {
			return installer.NewArgoCD("", dcId, ociPw, registryURL, "", true, false, "", nil)
		},
		NewArgoCDResources: func(dcId, ociPw, registryURL string) (ArgoCDResourcesApplier, error) {
			return installer.NewArgoCDResources(dcId, ociPw, registryURL, "")
		},
		NewVaultSecretsDeployer: func(c ctrlclient.Client) VaultSecretsDeployer {
			return installer.NewVaultSecretCreator(c)
		},
		NewPCAppsRunner: func(c ctrlclient.Client, version, namespace string) (PCAppsRunner, error) {
			return installer.NewPCApps(c, version, namespace, nil)
		},
	}
}

// Run executes the full ArgoCD integration sequence:
//  1. (Optional) Install the ArgoCD Helm chart.
//  2. (Optional) Deploy a SOPS-encrypted vault file as a Kubernetes secret.
//  3. Update the ArgoCD Helm OCI pull secret.
//  4. Install pc-apps at the version recorded in bom.json.
func Run(ctx context.Context, kubeClient ctrlclient.Client, opts Opts) error {
	return RunWithDeps(ctx, kubeClient, opts, defaultDeps())
}

// RunWithDeps is like Run but accepts injectable Deps for testing.
func RunWithDeps(ctx context.Context, kubeClient ctrlclient.Client, opts Opts, deps Deps) error {
	if opts.VaultNamespace == "" {
		opts.VaultNamespace = DefaultVaultNamespace
	}
	if opts.VaultSecretName == "" {
		opts.VaultSecretName = DefaultVaultSecretName
	}

	bomCfg, err := bom.Parse(opts.BomPath)
	if err != nil {
		return fmt.Errorf("failed to parse bom.json: %w", err)
	}

	pcAppsVersion, hasPCApps := bomCfg.GetPCAppsVersion()
	if !hasPCApps {
		log.Println("WARNING: pc-applications component not found in bom.json — skipping pc-apps install")
	}

	if opts.InstallArgoCD {
		log.Println("Installing ArgoCD...")
		argoCDInstaller, err := deps.NewArgoCDInstaller(opts.DatacenterID, opts.OCIPassword, opts.RegistryURL)
		if err != nil {
			return fmt.Errorf("failed to initialize ArgoCD installer: %w", err)
		}
		if err := argoCDInstaller.Install(); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
		log.Println("ArgoCD installed.")
	}

	if opts.VaultFile != "" {
		log.Printf("Deploying vault secrets from %s to %s/%s...", opts.VaultFile, opts.VaultNamespace, opts.VaultSecretName)
		deployer := deps.NewVaultSecretsDeployer(kubeClient)
		if err := deployer.CreateSecretFromVault(ctx, opts.VaultFile, opts.AgeKeyPath, opts.VaultNamespace, opts.VaultSecretName); err != nil {
			return fmt.Errorf("failed to deploy vault secrets: %w", err)
		}
	}

	log.Println("Updating ArgoCD OCI pull secret...")
	resources, err := deps.NewArgoCDResources(opts.DatacenterID, opts.OCIPassword, opts.RegistryURL)
	if err != nil {
		return fmt.Errorf("failed to initialize ArgoCD resources: %w", err)
	}
	if err := resources.ApplyAll(ctx); err != nil {
		return fmt.Errorf("failed to apply ArgoCD resources: %w", err)
	}
	log.Println("ArgoCD OCI pull secret updated.")

	if !hasPCApps {
		return nil
	}

	log.Printf("Installing pc-applications version %s...", pcAppsVersion)
	pcRunner, err := deps.NewPCAppsRunner(kubeClient, pcAppsVersion, "argocd")
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}
	if err := pcRunner.Install(ctx); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
	}
	log.Printf("pc-applications version %s installed successfully.", pcAppsVersion)

	return nil
}
