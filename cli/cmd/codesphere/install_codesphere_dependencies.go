// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallCodesphereDepenciesCmd runs the cluster dependency steps (Phase 2).
type InstallCodesphereDepenciesCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodesphereDepenciesCmd) RunE(_ *cobra.Command, _ []string) error {
	if err := validateInstallCodesphereVault(c.Opts); err != nil {
		return err
	}
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

	installVault, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("failed to get vault and Kubernetes config: %w", err)
	}

	scheme := k8sruntime.NewScheme()
	if err := k8sscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add Kubernetes core scheme: %w", err)
	}

	if err := argov1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add ArgoCD scheme: %w", err)
	}

	kubeClient, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	err = stlog.Step("Ensure Codesphere prerequisites", func() error {
		return installer.EnsureCodespherePrerequisites(context.Background(), kubeClient)
	})
	if err != nil {
		return fmt.Errorf("failed to ensure Codesphere prerequisites: %w", err)
	}

	if !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.ArgoCDStep) {
		err = ci.ExtractAndValidatePackage(pm)
		if err != nil {
			return fmt.Errorf("failed to extract and validate package: %w", err)
		}

		err = stlog.Step("Install ArgoCD pre-step", func() error {
			return installArgoCDAndApps(opts, cfg, pm, installVault, restConfig, kubeClient, stlog)
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
func installArgoCDAndApps(opts *InstallCodesphereOpts, cfg files.RootConfig, pm installer.PackageManager, installVault *files.InstallVault, restConfig *rest.Config, kubeClient ctrlclient.Client, stlog *bootstrap.StepLogger) error {
	bomConfig, err := bom.Parse(pm.GetDependencyPath("bom.json"))
	if err != nil {
		return fmt.Errorf("failed to parse installer BOM: %w", err)
	}
	configuredRegistryURL := ""
	if cfg.Registry != nil {
		configuredRegistryURL = strings.TrimSuffix(strings.TrimPrefix(cfg.Registry.Server, "oci://"), "/")
		if configuredRegistryURL != "" && configuredRegistryURL != "ghcr.io" {
			if err := bomConfig.UseRegistry(configuredRegistryURL); err != nil {
				return fmt.Errorf("failed to configure installer BOM registry: %w", err)
			}
		}
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
		registryURL := opts.ArgoCDRegistryURL
		if registryURL == "" && configuredRegistryURL != "" {
			registryURL = configuredRegistryURL + "/codesphere-cloud/charts"
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
			BOM:            bomConfig,
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
		return install.InstallPCApps(context.Background(), bomConfig)
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
