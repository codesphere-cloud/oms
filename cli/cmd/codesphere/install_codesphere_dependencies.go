// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
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

func (c *InstallCodesphereDepenciesCmd) RunE(_ *cobra.Command, _ []string) error {
	effectiveOpts, cfg, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	return installCodesphereDepencies(effectiveOpts, cfg, c.Env)
}

func installCodesphereDepencies(opts *InstallCodesphereOpts, cfg files.RootConfig, env env.Env) error {
	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	stlog := bootstrap.NewStepLogger(false)
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	ci := &installer.CodesphereInstaller{
		ConfigPath:       opts.ConfigPath,
		VaultPath:        opts.Vault,
		PrivKey:          opts.PrivKey,
		Force:            opts.Force,
		SkipSteps:        opts.SkipSteps,
		AllowedSteps:     installer.DependenciesSteps,
		DirectConnection: opts.DirectConnection,
		AutoApprove:      opts.AutoApprove,
	}

	if !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.ArgoCDStep) {
		if err := ci.ExtractAndValidatePackage(pm); err != nil {
			return fmt.Errorf("failed to extract and validate package: %w", err)
		}
		if err := stlog.Step("Install ArgoCD pre-step", func() error {
			return installArgoCDAndApps(opts, cfg, pm, stlog)
		}); err != nil {
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
func installArgoCDAndApps(opts *InstallCodesphereOpts, cfg files.RootConfig, pm installer.PackageManager, stlog *bootstrap.StepLogger) error {
	var install *argocdinstaller.AppInstaller
	if err := stlog.Substep("Load vault data", func() error {
		installVault, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, cfg)
		if err != nil {
			return err
		}
		registryPassword := ""
		if secret := installVault.GetSecret(files.SecretRegistryPassword); secret != nil && secret.Fields != nil {
			registryPassword = secret.Fields.Password
		}
		if registryPassword == "" {
			return fmt.Errorf("registry password not found in vault (secret %q)", files.SecretRegistryPassword)
		}
		kubeClient, err := ctrlclient.New(restConfig, ctrlclient.Options{})
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		registryURL := opts.ArgoCDRegistryURL
		if registryURL == "" && cfg.Registry != nil {
			registryURL = cfg.Registry.Server + "/codesphere-cloud/charts"
		}
		argoCDInstall, err := argocdinstaller.NewInstaller(argocdinstaller.InstallerConfig{
			Version:        opts.ArgoCDVersion,
			DatacenterId:   fmt.Sprintf("%d", cfg.Datacenter.ID),
			OciPassword:    registryPassword,
			OciRegistryURL: registryURL,
			GitPassword:    os.Getenv("OMS_GIT_PASSWORD"),
			FullInstall:    true,
			ForceConflicts: opts.ArgoCDForceConflicts,
			RepoURL:        opts.ArgoCDRepoURL,
			ValueFiles:     opts.ArgoCDValues,
			RESTConfig:     restConfig,
		})
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
		return install.SyncVaultSecret(context.Background())
	}); err != nil {
		return err
	}
	if err := stlog.Substep("Install pc-apps", func() error {
		return install.InstallPCApps(context.Background(), pm.GetDependencyPath("bom.json"))
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
