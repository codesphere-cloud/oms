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
	"github.com/codesphere-cloud/oms/internal/configtemplating"
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
	cm := installer.NewConfig()

	cfg, cleanup, err := parseInstallConfig(opts, cm)
	if err != nil {
		return fmt.Errorf("failed to extract config.yaml: %w", err)
	}
	defer cleanup()

	if !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.ArgoCDStep) {
		if err := stlog.Step("Install ArgoCD pre-step", func() error {
			return installArgoCDAndApps(opts, pm, stlog)
		}); err != nil {
			return err
		}
	}
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

func parseInstallConfig(opts *InstallCodesphereOpts, cm installer.ConfigManager) (files.RootConfig, func(), error) {
	configPath := opts.Config
	cleanup := func() {}
	if opts.Vault != "" {
		store := installer.NewLazyVaultTemplatingSecretStore(opts.Vault, opts.PrivKey)
		renderedConfig, renderCleanup, err := configtemplating.RenderConfigFileToTemp(opts.Config, store)
		if err != nil {
			return files.RootConfig{}, cleanup, err
		}
		configPath = renderedConfig
		cleanup = renderCleanup
	}

	cfg, err := cm.ParseConfigYaml(configPath)
	return cfg, cleanup, err
}

// installArgoCDAndApps runs ArgoCD install, vault secret sync, and pc-apps install
// before the main dependency steps.
func installArgoCDAndApps(opts *InstallCodesphereOpts, pm installer.PackageManager, stlog *bootstrap.StepLogger) error {
	install := &argoCDAndAppsInstall{
		ctx:  context.Background(),
		opts: opts,
		pm:   pm,
	}

	if err := stlog.Substep("Load vault data", install.loadVaultData); err != nil {
		return err
	}
	if err := stlog.Substep("Install ArgoCD", install.installArgoCD); err != nil {
		return err
	}
	if err := stlog.Substep("Sync vault secret", install.syncVaultSecret); err != nil {
		return err
	}
	if err := stlog.Substep("Install pc-apps", install.installPcApps); err != nil {
		return err
	}

	return nil
}

type argoCDAndAppsInstall struct {
	ctx context.Context

	opts *InstallCodesphereOpts
	pm   installer.PackageManager

	ociPassword    string
	ociRegistryURL string
	argoInstall    *argocdinstaller.Installer
	kubeClient     ctrlclient.Client
	vault          *files.InstallVault
	kubeConfig     *rest.Config
}

func (i *argoCDAndAppsInstall) loadVaultData() error {
	var err error
	i.vault, err = installer.LoadVaultData(i.opts.Vault, i.opts.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to load vault: %w", err)
	}
	if s := i.vault.GetSecret(files.SecretRegistryPassword); s != nil && s.Fields != nil {
		i.ociPassword = s.Fields.Password
	}
	if i.ociPassword == "" {
		return fmt.Errorf("registry password not found in vault (secret %q)", files.SecretRegistryPassword)
	}
	kubeConfigContent, err := kubeConfigContentFromVault(i.vault)
	if err != nil {
		return err
	}

	i.kubeConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfigContent))
	if err != nil {
		return fmt.Errorf("failed to load kubernetes config from vault: %w", err)
	}
	i.kubeClient, err = ctrlclient.New(i.kubeConfig, ctrlclient.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return nil
}

func (i *argoCDAndAppsInstall) installArgoCD() error {
	cfg, err := installer.NewConfig().ParseConfigYaml(i.opts.Config)
	if err != nil {
		return fmt.Errorf("failed to parse config.yaml: %w", err)
	}
	i.ociRegistryURL = i.opts.ArgoCDRegistryURL
	if i.ociRegistryURL == "" && cfg.Registry != nil {
		i.ociRegistryURL = cfg.Registry.Server
	}
	i.argoInstall, err = argocdinstaller.NewInstaller(argocdinstaller.InstallerConfig{
		Version:        i.opts.ArgoCDVersion,
		DatacenterId:   fmt.Sprintf("%d", cfg.Datacenter.ID),
		OciPassword:    i.ociPassword,
		OciRegistryURL: i.ociRegistryURL,
		GitPassword:    os.Getenv("OMS_GIT_PASSWORD"),
		FullInstall:    true,
		ForceConflicts: i.opts.ArgoCDForceConflicts,
		RepoURL:        i.opts.ArgoCDRepoURL,
		ValueFiles:     i.opts.ArgoCDValues,
		RESTConfig:     i.kubeConfig,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize ArgoCD installer: %w", err)
	}
	if err := i.argoInstall.Install(); err != nil {
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}
	return nil
}

func (i *argoCDAndAppsInstall) syncVaultSecret() error {
	creator := installer.NewVaultSecretCreator(i.kubeClient)
	if err := creator.CreateSecretFromVault(i.ctx, i.vault, installer.VaultSecretNamespace, installer.VaultSecretName); err != nil {
		return fmt.Errorf("failed to sync vault secret: %w", err)
	}
	return nil
}

func (i *argoCDAndAppsInstall) installPcApps() error {
	pcApps, err := installer.NewPcAppsFromBom(i.kubeClient, i.pm.GetDependencyPath("bom.json"), argocdinstaller.DefaultNamespace)
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}
	if err := pcApps.Install(i.ctx); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
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
			Pass --skip-steps argocd or add argocd to operations.skip to skip the ArgoCD pre-step.`),
			Example: formatExamples("install codesphere dependencies", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install cluster dependencies (including ArgoCD)",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml -s argocd",
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
