// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

const (
	mergedInstallConfigDirPattern = "oms-install-config-*"
	mergedInstallConfigFileName   = "config.yaml"
)

// InstallCodesphereCmd represents the codesphere command
type InstallCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

type InstallCodesphereOpts struct {
	*GlobalOptions
	Package          string
	Force            bool
	Configs          []string
	ConfigPath       string
	Vault            string
	PrivKey          string
	SkipSteps        []string
	CodesphereOnly   bool
	DirectConnection bool
	AutoApprove      bool
	// ArgoCD deployment (pre-step in Phase 2)
	ArgoCDVersion        string
	ArgoCDRegistryURL    string
	ArgoCDForceConflicts bool
	ArgoCDRepoURL        string
	ArgoCDValues         []string
	PCAppsValues         []string
}

func (c *InstallCodesphereCmd) RunE(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	effectiveOpts, cfg, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	infraInstaller := &installer.CodesphereInstaller{
		SkipSteps:    c.Opts.SkipSteps,
		AllowedSteps: installer.InfraSteps,
	}
	dependenciesInstaller := &installer.CodesphereInstaller{
		SkipSteps:    append(sharedInstallCodesphereSteps(), c.Opts.SkipSteps...),
		AllowedSteps: installer.DependenciesSteps,
	}
	platformInstaller := &installer.CodesphereInstaller{
		SkipSteps:    append(sharedInstallCodesphereSteps(), c.Opts.SkipSteps...),
		AllowedSteps: installer.PlatformSteps,
	}

	if c.Opts.CodesphereOnly {
		return installCodespherePlatform(ctx, effectiveOpts, cfg, c.Env)
	}

	if infraInstaller.HasExecutableSteps(cfg) {
		if err := installCodesphereInfra(effectiveOpts, c.Env); err != nil {
			return err
		}
	}

	if dependenciesInstaller.HasExecutableSteps(cfg) || !installer.IsStepSkipped(cfg, c.Opts.SkipSteps, installer.ArgoCDStep) {
		if err := installCodesphereDepencies(effectiveOpts, cfg, c.Env); err != nil {
			return err
		}
	}

	if !platformInstaller.HasExecutableSteps(cfg) {
		return nil
	}

	return installCodespherePlatform(ctx, effectiveOpts, cfg, c.Env)
}

func AddInstallCodesphereCmd(install *cobra.Command, opts *GlobalOptions) {
	codesphere := InstallCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Install a Codesphere instance",
			Long: io.Long(`Install a Codesphere instance with the provided package, configuration file, and private key.
			Uses the private-cloud-installer.js script included in the package to perform the installation.`),
			Example: formatExamples("install codesphere", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s copy-dependencies,extract-dependencies,load-container-images,ceph,postgres,kubernetes,docker",
					Desc: "Skip most pre-installation steps. E.g. if you only need to re-apply Codesphere's helm charts",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images",
					Desc: "Skip loading container images. Necessary when installing a lite package that doesn't include any container images",
				},
			}),
		},
		Opts: &InstallCodesphereOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load binaries, installer etc. from")
	codesphere.cmd.PersistentFlags().BoolVarP(&codesphere.Opts.Force, "force", "f", false, "Enforce package extraction")
	codesphere.cmd.PersistentFlags().StringArrayVarP(&codesphere.Opts.Configs, "config", "c", nil, "Path to a Codesphere Private Cloud configuration file (yaml). Can be specified multiple times and merged in order")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.Vault, "vault", "", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	codesphere.cmd.PersistentFlags().StringSliceVarP(&codesphere.Opts.SkipSteps, "skip-steps", "s", []string{}, "Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.CodesphereOnly, "codesphere-only", false, "Install only Codesphere without dependencies")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDVersion, "argo-version", "", "ArgoCD Helm chart version to install")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRegistryURL, "argo-registry-url", "", "OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.ArgoCDForceConflicts, "argo-force-conflicts", false, "Force SSA ownership conflicts during ArgoCD install")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRepoURL, "argo-repo", argocd.DefaultRepoURL, "ArgoCD Helm chart repository URL")
	codesphere.cmd.PersistentFlags().StringArrayVar(&codesphere.Opts.ArgoCDValues, "argo-values", nil, "ArgoCD values YAML file (can be specified multiple times)")
	codesphere.cmd.PersistentFlags().StringArrayVar(&codesphere.Opts.PCAppsValues, "pc-apps-values", nil, "pc-apps values YAML file (can be specified multiple times)")

	util.MarkPersistentFlagRequired(codesphere.cmd, "package")
	util.MarkPersistentFlagRequired(codesphere.cmd, "config")
	util.MarkPersistentFlagRequired(codesphere.cmd, "priv-key")

	AddCmd(install, codesphere.cmd)

	codesphere.cmd.RunE = codesphere.RunE

	AddInstallCodesphereInfraCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodesphereDepenciesCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodespherePlatformCmd(codesphere.cmd, codesphere.Opts)
}

func sharedInstallCodesphereSteps() []string {
	return []string{"copy-dependencies", "extract-dependencies"}
}

// prepareInstallConfig resolves the install command's repeated --config inputs
// into a single config file for downstream installer steps.
//
// For each input config it optionally renders vault-backed template expressions,
// parses the YAML into a generic map, and deep-merges the maps in flag order so
// later --config values override earlier ones. The merged YAML is then written
// to a dedicated temporary directory at a stable file name,
// "<tmp>/config.yaml", parsed once through the config manager, and returned via
// effectiveOpts.ConfigPath. The returned cleanup function removes any rendered
// per-file temps as well as the merged config directory.
func prepareInstallConfig(opts *InstallCodesphereOpts, cm installer.ConfigManager) (*InstallCodesphereOpts, files.RootConfig, func(), error) {
	configFiles := append([]string(nil), opts.Configs...)
	if len(configFiles) == 0 {
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("no config.yaml input provided: at least one config file is required")
	}

	store := installer.NewLazyVaultTemplatingSecretStore(opts.Vault, opts.PrivKey)
	cleanupFns := []func(){}
	cleanup := func() {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
	}

	merged := map[string]any{}
	for _, configPath := range configFiles {
		renderedPath := configPath
		if opts.Vault != "" {
			tmpPath, renderCleanup, err := configtemplating.RenderConfigFileToTemp(configPath, store)
			if err != nil {
				cleanup()
				return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to render config template %s: %w", configPath, err)
			}
			cleanupFns = append(cleanupFns, renderCleanup)
			renderedPath = tmpPath
		}

		data, err := os.ReadFile(renderedPath)
		if err != nil {
			cleanup()
			return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to read config file %s: %w", renderedPath, err)
		}

		var partial map[string]any
		if err := yaml.Unmarshal(data, &partial); err != nil {
			cleanup()
			return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to parse config file %s: %w", renderedPath, err)
		}
		if partial == nil {
			partial = map[string]any{}
		}
		merged = util.DeepMergeMaps(merged, partial)
	}

	mergedBytes, err := yaml.Marshal(merged)
	if err != nil {
		cleanup()
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to marshal merged config.yaml: %w", err)
	}

	mergedDir, err := os.MkdirTemp("", mergedInstallConfigDirPattern)
	if err != nil {
		cleanup()
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to create merged config directory: %w", err)
	}
	mergedPath := filepath.Join(mergedDir, mergedInstallConfigFileName)
	tmp, err := os.OpenFile(mergedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		cleanup()
		_ = os.RemoveAll(mergedDir)
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to create merged config file %s: %w", mergedPath, err)
	}
	if _, err := tmp.Write(mergedBytes); err != nil {
		_ = tmp.Close()
		_ = os.RemoveAll(mergedDir)
		cleanup()
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to write merged config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.RemoveAll(mergedDir)
		cleanup()
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to close merged config file: %w", err)
	}
	cleanupFns = append(cleanupFns, func() {
		_ = os.RemoveAll(mergedDir)
	})

	cfg, err := cm.ParseConfigYaml(mergedPath)
	if err != nil {
		cleanup()
		return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to parse merged config.yaml: %w", err)
	}

	effectiveOpts := *opts
	effectiveOpts.ConfigPath = mergedPath
	effectiveOpts.Configs = append([]string(nil), configFiles...)

	return &effectiveOpts, cfg, cleanup, nil
}
