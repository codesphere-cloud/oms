// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

const (
	secretRefMarker               = "__oms_secret_ref__"
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
	*util.GlobalOptions
	Package          string
	Force            bool
	Configs          []string
	ConfigPath       string
	Vault            string
	PrivKey          string
	VaultType        string
	SkipSteps        []string
	CodesphereOnly   bool
	DirectConnection bool
	AutoApprove      bool
	StepTimer        bool
	LocalComponents  bool
	LocalConfigDir   string
	// ArgoCD deployment (pre-step in Phase 2)
	ArgoCDRegistryURL    string
	ArgoCDRepoURL        string
	ArgoCDVersion        string
	ArgoCDRegistryCA     string
	ArgoCDForceConflicts bool
	ArgoCDValues         []string
	PCAppsValues         []string
	// Name of the k8s secret containing the entire vault; source for ESO.
	VaultSecretName string
}

func (c *InstallCodesphereCmd) RunE(cmd *cobra.Command, _ []string) error {
	if c.Opts.LocalComponents {
		return fmt.Errorf("--local-components requires install codesphere dependencies or install codesphere platform")
	}
	if err := validateInstallCodesphereVault(c.Opts); err != nil {
		return err
	}
	ctx := cmd.Context()
	effectiveOpts, cfg, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	if err := ensureCodespherePackageForVersion(effectiveOpts, cfg, c.Env); err != nil {
		return err
	}

	infraInstaller := &installer.CodesphereInstaller{
		SkipSteps:    c.Opts.SkipSteps,
		AllowedSteps: installer.InfraSteps,
		Verbose:      c.Opts.Verbose,
	}
	dependenciesInstaller := &installer.CodesphereInstaller{
		SkipSteps:    append(sharedInstallCodesphereSteps(), c.Opts.SkipSteps...),
		AllowedSteps: installer.DependenciesSteps,
		Verbose:      c.Opts.Verbose,
	}
	platformInstaller := &installer.CodesphereInstaller{
		SkipSteps:    append(sharedInstallCodesphereSteps(), c.Opts.SkipSteps...),
		AllowedSteps: installer.PlatformSteps,
		Verbose:      c.Opts.Verbose,
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
		if err := installCodesphereDepencies(ctx, effectiveOpts, cfg, c.Env); err != nil {
			return err
		}
	}

	if !platformInstaller.HasExecutableSteps(cfg) {
		return nil
	}

	return installCodespherePlatform(ctx, effectiveOpts, cfg, c.Env)
}

func AddInstallCmd(install *cobra.Command, opts *util.GlobalOptions) {
	codesphere := InstallCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Install a Codesphere instance",
			Long: io.Long(`Install a Codesphere instance with the provided configuration file and private key.
			When --package is omitted, operations.codesphere selects the version to download
			and reuse in OMS_WORKDIR. An explicit --package overrides the configured version.
			Uses the private-cloud-installer.js script included in the package to perform the installation.
			Use the dependencies and platform subcommands with --local-components for a prepared local cluster.`),
			Example: util.FormatExamples("install codesphere", []io.Example{
				{Cmd: "-c config.yaml -k <path-to-private-key>", Desc: "Install the operations.codesphere version, downloading it to OMS_WORKDIR if needed"},
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
		Opts: &InstallCodesphereOpts{GlobalOptions: opts, VaultType: string(vault.TypeSOPS)},
		Env:  env.NewEnv(),
	}
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.Package, "package", "p", "", "Package file to use. This flag overrides the version specified in the configuration file.")
	codesphere.cmd.PersistentFlags().BoolVarP(&codesphere.Opts.Force, "force", "f", false, "Enforce package extraction")
	codesphere.cmd.PersistentFlags().StringArrayVarP(&codesphere.Opts.Configs, "config", "c", nil, "Path to a Codesphere Private Cloud configuration file (yaml). Can be specified multiple times and merged in order")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.Vault, "vault", "", "Path to the prod.vault.yaml file used for config templating")
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Path to the age private key (required for sops unless an age key environment variable is set)")
	codesphere.cmd.PersistentFlags().StringSliceVarP(&codesphere.Opts.SkipSteps, "skip-steps", "s", []string{}, "Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.StepTimer, util.TimerFlag, false, util.ShowStepDurationHelp)
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.LocalComponents, "local-components", false, "Run selected install components on the local host")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.LocalConfigDir, "local-config-dir", "", "Config directory for local install components")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.CodesphereOnly, "codesphere-only", false, "Install only Codesphere without dependencies")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDVersion, "argo-version", "", "Upstream ArgoCD Helm chart version; overrides the packaged chart")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRegistryURL, "argo-registry-url", "", "OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRegistryCA, "argo-registry-ca", "", "Path to the certificate authority of the OCI registry, needed when its certificate is self-signed")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.ArgoCDForceConflicts, "argo-force-conflicts", false, "Force SSA ownership conflicts during ArgoCD install")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRepoURL, "argo-repo", argocd.DefaultRepoURL, "Upstream ArgoCD chart repository; requires --argo-version")
	codesphere.cmd.PersistentFlags().StringArrayVar(&codesphere.Opts.ArgoCDValues, "argo-values", nil, "ArgoCD values YAML file; packaged chart settings go under argo-cd (repeatable)")
	codesphere.cmd.PersistentFlags().StringArrayVar(&codesphere.Opts.PCAppsValues, "pc-apps-values", nil, "pc-apps values YAML file (can be specified multiple times)")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.VaultSecretName, "vault-secret-name", vault.VaultSecretName, "Secret in the codesphere namespace the vault is written to (as source for ESO).")

	util.MarkPersistentFlagRequired(codesphere.cmd, "config")

	util.AddCmd(install, codesphere.cmd)

	codesphere.cmd.RunE = codesphere.RunE

	AddInstallCodesphereInfraCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodesphereDepenciesCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodespherePlatformCmd(codesphere.cmd, codesphere.Opts)
}

// validateInstallCodesphereVault enforces the current TypeScript installer
// contract without changing the selected type on the command options.
func validateInstallCodesphereVault(opts *InstallCodesphereOpts) error {
	if opts.VaultType != string(vault.TypeSOPS) {
		return fmt.Errorf("install codesphere requires vault type %q", vault.TypeSOPS)
	}

	err := vault.ValidateConfiguration(vault.TypeSOPS, opts.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to validate install config: %w", err)
	}

	if err := vault.ValidateSecretName(opts.VaultSecretName); err != nil {
		return fmt.Errorf("failed to validate --vault-secret-name: %w", err)
	}

	return nil
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

	var store *vault.VaultTemplatingSecretStore

	if opts.Vault != "" {
		if err := checkNoCodesphereSecretRefs(configFiles); err != nil {
			return nil, files.RootConfig{}, func() {}, err
		}

		backend, err := vault.NewFromString(opts.VaultType, vault.Options{Path: opts.Vault, AgeKey: opts.PrivKey})
		if err != nil {
			return nil, files.RootConfig{}, func() {}, fmt.Errorf("failed to load vault: %w", err)
		}

		store = vault.NewLazyVaultTemplatingSecretStoreWithVault(backend)
	}
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
		// Normalize legacy versions so an object overlay can supply only a hash.
		if operations, ok := partial["operations"].(map[string]any); ok {
			if version, ok := operations["codesphere"].(string); ok {
				operations["codesphere"] = map[string]any{"version": version, "hash": ""}
			}
		}
		merged = intutil.DeepMergeMaps(merged, partial)
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

// checkNoCodesphereSecretRefs merges the configs like prepareInstallConfig does, because an
// overlay can set the release name that exempts the base config's secrets.
func checkNoCodesphereSecretRefs(configFiles []string) error {
	merged := map[string]any{}

	for _, configPath := range configFiles {
		config, err := readConfigWithSecretMarkers(configPath)
		if err != nil {
			return err
		}

		merged = intutil.DeepMergeMaps(merged, config)
	}

	if refs := codesphereSecretRefs(merged); len(refs) > 0 {
		return fmt.Errorf("invalid install config: secret templates are not supported in the codesphere block: %s", strings.Join(refs, ", "))
	}

	return nil
}

// codesphereSecretRefs returns the keys in the codesphere block that use `secret`. Their values
// reach the Codesphere chart's ExternalSecrets in plaintext. Dev releases, the only ones that set
// codesphere.releaseName, are exempt for now (only use case atm.). Long term config and secrets
// should be more strictly separated, i.e. no secrets in config.yml (requires some changes in
// auth-service).
func codesphereSecretRefs(config map[string]any) []string {
	codesphere, ok := config["codesphere"].(map[string]any)
	if !ok {
		return nil
	}

	if releaseName, _ := codesphere["releaseName"].(string); releaseName != "" {
		return nil
	}

	refs := []string{}

	for _, key := range slices.Sorted(maps.Keys(codesphere)) {
		if strings.Contains(fmt.Sprint(codesphere[key]), secretRefMarker) {
			refs = append(refs, "codesphere."+key)
		}
	}

	return refs
}

func readConfigWithSecretMarkers(configPath string) (map[string]any, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	rendered, err := configtemplating.RenderWithSecretResolver(data, func(string, ...string) (string, error) {
		return secretRefMarker, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render secret markers in %s: %w", configPath, err)
	}

	config := map[string]any{}
	if err := yaml.Unmarshal(rendered, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	return config, nil
}

// ensureCodespherePackageForVersion preserves explicit packages and resolves config-selected
// packages before any installation phase starts.
func ensureCodespherePackageForVersion(opts *InstallCodesphereOpts, cfg files.RootConfig, environment env.Env) error {
	if opts.Package != "" {
		return nil
	}

	version, err := cfg.CodespherePackageVersion()
	if err != nil {
		return fmt.Errorf("failed to resolve install package version: %w", err)
	}

	opts.Package, err = portal.EnsureCodespherePackage(portal.NewPortalClient(), environment.GetOmsWorkdir(), version, cfg.Operations.Codesphere.Hash, "installer-lite.tar.gz", false)
	if err != nil {
		return fmt.Errorf("failed to prepare install package: %w", err)
	}

	return nil
}
