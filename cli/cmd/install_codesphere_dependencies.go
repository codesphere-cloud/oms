// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallCodesphereDepenciesCmd runs the cluster dependency steps (Phase 2).
type InstallCodesphereDepenciesCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodesphereDepenciesCmd) RunE(_ *cobra.Command, _ []string) error {
	return installCodesphereDepencies(c.Opts, c.Env)
}

func installCodesphereDepencies(opts *InstallCodesphereOpts, env env.Env) error {
	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	stlog := bootstrap.NewStepLogger(false)

	if !opts.SkipArgo {
		if err := stlog.Step("Install ArgoCD pre-step", func() error {
			return installArgoCDAndApps(opts, pm, stlog)
		}); err != nil {
			return err
		}
	}
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	ci := &installer.CodesphereInstaller{
		ConfigPath:       opts.Config,
		VaultPath:        opts.Vault,
		PrivKey:          opts.PrivKey,
		Force:            opts.Force,
		SkipSteps:        opts.SkipSteps,
		AllowedSteps:     installer.DependenciesSteps,
		DirectConnection: opts.DirectConnection,
		AutoApprove:      opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}
	return nil
}

// installArgoCDAndApps runs ArgoCD install, vault secret sync, and pc-apps install
// before the main dependency steps.
func installArgoCDAndApps(opts *InstallCodesphereOpts, pm installer.PackageManager, stlog *bootstrap.StepLogger) error {
	ctx := context.Background()

	var (
		ociPassword    string
		ociRegistryURL string
		argoInstall    *argocdinstaller.Installer
		kubeClient     ctrlclient.Client
		vault          *files.InstallVault
		kubeConfig     *rest.Config
	)

	if err := stlog.Substep("Load vault data", func() error {
		var err error
		vault, err = installer.LoadVaultData(opts.Vault, opts.PrivKey)
		if err != nil {
			return fmt.Errorf("failed to load vault: %w", err)
		}
		if s := vault.GetSecret(files.SecretRegistryPassword); s != nil && s.Fields != nil {
			ociPassword = s.Fields.Password
		}
		if ociPassword == "" {
			return fmt.Errorf("registry password not found in vault (secret %q)", files.SecretRegistryPassword)
		}
		kubeConfigContent, err := kubeConfigContentFromVault(vault)
		if err != nil {
			return err
		}

		kubeConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfigContent))
		if err != nil {
			return fmt.Errorf("failed to load kubernetes config from vault: %w", err)
		}
		kubeClient, err = ctrlclient.New(kubeConfig, ctrlclient.Options{})
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := stlog.Substep("Install ArgoCD", func() error {
		cfg, err := installer.NewConfig().ParseConfigYaml(opts.Config)
		if err != nil {
			return fmt.Errorf("failed to parse config.yaml: %w", err)
		}
		ociRegistryURL = opts.ArgoCDRegistryURL
		if ociRegistryURL == "" && cfg.Registry != nil {
			ociRegistryURL = cfg.Registry.Server
		}
		argoInstall, err = argocdinstaller.NewInstaller(argocdinstaller.InstallerConfig{
			Version:        opts.ArgoCDVersion,
			DatacenterId:   fmt.Sprintf("%d", cfg.Datacenter.ID),
			OciPassword:    ociPassword,
			OciRegistryURL: ociRegistryURL,
			GitPassword:    os.Getenv("OMS_GIT_PASSWORD"),
			FullInstall:    true,
			ForceConflicts: opts.ArgoCDForceConflicts,
			RepoURL:        opts.ArgoCDRepoURL,
			ValueFiles:     opts.ArgoCDValues,
			RESTConfig:     kubeConfig,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize ArgoCD installer: %w", err)
		}
		if err := argoInstall.Install(); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := stlog.Substep("Sync vault secret", func() error {
		creator := installer.NewVaultSecretCreator(kubeClient)
		if err := creator.CreateSecretFromVault(ctx, vault, installer.VaultSecretNamespace, installer.VaultSecretName); err != nil {
			return fmt.Errorf("failed to sync vault secret: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := stlog.Substep("Install pc-apps", func() error {
		pcApps, err := installer.NewPcAppsFromBom(kubeClient, pm.GetDependencyPath("bom.json"), argocdinstaller.DefaultNamespace)
		if err != nil {
			return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
		}
		if err := pcApps.Install(ctx); err != nil {
			return fmt.Errorf("failed to install pc-apps: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func kubeConfigContentFromVault(vault *files.InstallVault) (string, error) {
	if vault == nil {
		return "", fmt.Errorf("vault is not loaded")
	}
	kubeConfig := vault.GetSecret(files.SecretKubeConfig)
	if kubeConfig == nil || kubeConfig.File == nil || strings.TrimSpace(kubeConfig.File.Content) == "" {
		return "", fmt.Errorf("kubeconfig not found in vault (secret %q)", files.SecretKubeConfig)
	}
	return kubeConfig.File.Content, nil
}

func AddInstallCodesphereDepenciesCmd(codesphere *cobra.Command, opts *InstallCodesphereOpts) {
	deps := InstallCodesphereDepenciesCmd{
		cmd: &cobra.Command{
			Use:   "dependencies",
			Short: "Install Codesphere cluster dependencies (Phase 2)",
			Long: io.Long(`Install cluster dependencies for a Codesphere instance (Phase 2).
			Runs ArgoCD install, vault secret sync, and pc-apps deployment first, then steps: set-up-cluster, ms-backends.
			Requires the infrastructure phase to have completed successfully.
			Pass --skip-argo to skip the ArgoCD pre-step.`),
			Example: formatExamples("install codesphere dependencies", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install cluster dependencies (including ArgoCD)",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml --skip-argo",
					Desc: "Install cluster dependencies without the ArgoCD pre-step",
				},
			}),
		},
		Opts: opts,
		Env:  env.NewEnv(),
	}

	AddCmd(codesphere, deps.cmd)
	deps.cmd.RunE = deps.RunE
}
