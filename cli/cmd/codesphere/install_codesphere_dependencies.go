// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallCodesphereDepenciesCmd runs the cluster dependency steps (Phase 2).
type InstallCodesphereDepenciesCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

// RunE installs the cluster dependencies selected by the command options.
func (c *InstallCodesphereDepenciesCmd) RunE(cmd *cobra.Command, _ []string) error {
	if err := validateInstallCodesphereVault(c.Opts); err != nil {
		return err
	}

	if c.Opts.VaultSecretName != vault.VaultSecretName {
		return fmt.Errorf("at the moment --vault-secret-name is only supported by the platform phase, the dependencies phase hardcodes %q", vault.VaultSecretName)
	}

	if c.Opts.LocalComponents && c.Opts.LocalConfigDir == "" {
		return fmt.Errorf("--local-config-dir is required with --local-components")
	}
	effectiveOpts, cfg, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	if err := ensureCodespherePackageForVersion(effectiveOpts, cfg, c.Env); err != nil {
		return err
	}

	return installCodesphereDepencies(cmd.Context(), effectiveOpts, cfg, c.Env)
}

func installCodesphereDepencies(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig, env env.Env) error {
	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	if opts.LocalComponents {
		pm = installer.NewPackage(filepath.Dir(opts.Package), opts.Package)
	}
	stlog := bootstrap.NewStepLogger(false, bootstrap.WithTimer(opts.StepTimer))
	defer stlog.PrintSummary()
	cm := installer.NewConfig()
	im := system.NewImage(ctx)

	ci := &installer.CodesphereInstaller{
		ConfigPath:        opts.ConfigPath,
		VaultPath:         opts.Vault,
		PrivKey:           opts.PrivKey,
		Force:             opts.Force,
		Verbose:           opts.Verbose,
		SkipSteps:         opts.SkipSteps,
		AllowedSteps:      installer.DependenciesSteps,
		DirectConnection:  opts.DirectConnection,
		AutoApprove:       opts.AutoApprove,
		LocalComponents:   opts.LocalComponents,
		LocalConfigDir:    opts.LocalConfigDir,
		SkipImageBuilding: opts.LocalComponents,
		Context:           ctx,
	}

	_, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("failed to get vault and Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	err = stlog.Step("Ensure Codesphere prerequisites", func() error {
		return installer.EnsureCodespherePrerequisites(ctx, kubeClient)
	})
	if err != nil {
		return fmt.Errorf("failed to ensure Codesphere prerequisites: %w", err)
	}

	if !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.ArgoCDStep) {
		err = ci.ExtractAndValidatePackage(pm)
		if err != nil {
			return fmt.Errorf("failed to extract and validate package: %w", err)
		}

		appsConfig := cfg
		if opts.LocalComponents {
			// The temporary NodePort is only for local component migrations.
			appsConfig.Codesphere.Migration = nil
		}
		err = stlog.Step("Install ArgoCD pre-step", func() error {
			return installArgoCDAndApps(ctx, opts, appsConfig, pm, kubeClient, stlog)
		})
		if err != nil {
			return err
		}
	}

	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}
	return nil
}

// installArgoCDAndApps runs ArgoCD install, vault secret sync, and pc-apps install
// before the main dependency steps.
func installArgoCDAndApps(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig, pm installer.PackageManager, kubeClient ctrlclient.Client, stlog *bootstrap.StepLogger) error {
	bomConfig, err := bom.Parse(pm.GetDependencyPath("bom.json"))
	if err != nil {
		return fmt.Errorf("failed to parse installer BOM: %w", err)
	}

	registryURL := ""
	if cfg.Registry != nil {
		registryURL = cfg.Registry.Server + installer.ChartsRepositoryPath
	}

	var install *argocdinstaller.AppInstaller

	if err := stlog.Substep("Load vault data", func() error {
		installVault, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
		if err != nil {
			return fmt.Errorf("failed to load vault data and REST config: %w", err)
		}
		registryPassword := ""
		if secret := installVault.GetSecret(files.SecretRegistryPassword); secret != nil && secret.Fields != nil {
			registryPassword = secret.Fields.Password
		}
		if registryPassword == "" {
			return fmt.Errorf("registry password not found in vault (secret %q)", files.SecretRegistryPassword)
		}
		// A mirrored registry has a user of its own, so ArgoCD is given the same credentials the
		// rest of the installation pulls with instead of the GHCR default.
		registryUsername := ""
		if secret := installVault.GetSecret(files.SecretRegistryUsername); secret != nil && secret.Fields != nil {
			registryUsername = secret.Fields.Password
		}

		argoCDInstall, err := argocdinstaller.NewInstaller(argocdinstaller.InstallerConfig{
			DatacenterID:      fmt.Sprintf("%d", cfg.Datacenter.ID),
			OciUsername:       registryUsername,
			OciPassword:       registryPassword,
			OciRegistryURL:    registryURL,
			OciRegistryCAFile: opts.ArgoCDRegistryCA,
			GitPassword:       os.Getenv("OMS_GIT_PASSWORD"),
			FullInstall:       true,
			ForceConflicts:    opts.ArgoCDForceConflicts,
			ValueFiles:        opts.ArgoCDValues,
			RESTConfig:        restConfig,
		}, bomConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize ArgoCD installer: %w", err)
		}
		install = argocdinstaller.NewAppInstaller(argocdinstaller.AppInstallerConfig{
			Config:       cfg,
			Vault:        installVault,
			RESTConfig:   restConfig,
			KubeClient:   kubeClient,
			Installer:    argoCDInstall,
			PCAppsValues: opts.PCAppsValues,
		})
		return nil
	}); err != nil {
		return err
	}
	if err := stlog.Substep("Install ArgoCD", install.InstallArgoCD); err != nil {
		return err
	}
	if err := stlog.Substep("Sync vault secret", func() error {
		return install.SyncVaultSecret(ctx)
	}); err != nil {
		return err
	}
	if err := stlog.Substep("Install pc-apps", func() error {
		return install.InstallPCApps(ctx, bomConfig)
	}); err != nil {
		return err
	}

	return nil
}

func AddInstallCodesphereDepenciesCmd(codesphere *cobra.Command, opts *InstallCodesphereOpts) {
	deps := InstallCodesphereDepenciesCmd{
		cmd: &cobra.Command{
			Use:   "dependencies",
			Short: "Install Codesphere cluster dependencies (Phase 2)",
			Long: io.Long(`Install cluster dependencies for a Codesphere instance (Phase 2).
			Runs ArgoCD install, vault secret sync, and pc-apps deployment first, then steps: set-up-cluster, ms-backends.
			Requires the infrastructure phase to have completed successfully.
			Pass --skip-steps argocd or add argocd to operations.skip to skip the ArgoCD pre-step.`),
			Example: util.FormatExamples("install codesphere dependencies", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install cluster dependencies (including ArgoCD)",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s argocd",
					Desc: "Install cluster dependencies without the ArgoCD pre-step",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml --pc-apps-values base.yaml --pc-apps-values dc-overlay.yaml",
					Desc: "Install cluster dependencies with custom pc-apps values files",
				},
			}),
		},
		Opts: opts,
		Env:  env.NewEnv(),
	}

	util.AddCmd(codesphere, deps.cmd)
	deps.cmd.RunE = deps.RunE
}
