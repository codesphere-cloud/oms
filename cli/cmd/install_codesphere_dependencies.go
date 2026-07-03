// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
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
	install := &argoCDAndAppsInstall{
		ctx:    context.Background(),
		opts:   opts,
		pm:     pm,
		config: cfg,
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
	config         files.RootConfig
}

func (i *argoCDAndAppsInstall) loadVaultData() error {
	vaultPath, err := i.resolveVaultPath()
	if err != nil {
		return err
	}

	i.vault, err = installer.LoadVaultData(vaultPath, i.opts.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to load vault %s: %w", vaultPath, err)
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

func (i *argoCDAndAppsInstall) resolveVaultPath() (string, error) {
	return resolveVaultPath(i.opts.Vault, i.config)
}

// resolveVaultPath returns the explicit --vault path or falls back to
// prod.vault.yaml in the config's secrets baseDir.
func resolveVaultPath(vaultPath string, config files.RootConfig) (string, error) {
	if strings.TrimSpace(vaultPath) != "" {
		return vaultPath, nil
	}
	if strings.TrimSpace(config.Secrets.BaseDir) == "" {
		return "", fmt.Errorf("vault path is not set and config.yaml secrets.baseDir is empty")
	}
	return filepath.Join(config.Secrets.BaseDir, "prod.vault.yaml"), nil
}

func (i *argoCDAndAppsInstall) installArgoCD() error {
	i.ociRegistryURL = i.opts.ArgoCDRegistryURL
	if i.ociRegistryURL == "" && i.config.Registry != nil {
		i.ociRegistryURL = i.config.Registry.Server + "/codesphere-cloud/charts"
	}
	var err error
	i.argoInstall, err = argocdinstaller.NewInstaller(argocdinstaller.InstallerConfig{
		Version:        i.opts.ArgoCDVersion,
		DatacenterId:   fmt.Sprintf("%d", i.config.Datacenter.ID),
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
	// Always create new service accounts tokens during creation to ensure they are always valid and updated.
	if err := secrets.EnsureServiceAccountTokens(i.vault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}
	creator := installer.NewVaultSecretCreator(i.kubeClient)
	if err := creator.CreateSecretFromVault(i.ctx, i.vault, installer.VaultSecretNamespace, installer.VaultSecretName); err != nil {
		return fmt.Errorf("failed to sync vault secret: %w", err)
	}
	return nil
}

func (i *argoCDAndAppsInstall) installPcApps() error {
	pcApps, err := installer.NewPcAppsFromBom(
		i.kubeClient,
		i.kubeConfig,
		i.pm.GetDependencyPath("bom.json"),
		argocdinstaller.DefaultNamespace,
		i.opts.PCAppsValues,
		i.config.PcApps,
	)
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

	AddCmd(codesphere, deps.cmd)
	deps.cmd.RunE = deps.RunE
}
