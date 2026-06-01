// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

// knownInstallerSteps mirrors SKIPPABLE_STEPS from private-cloud-installer.ts.
var knownInstallerSteps = []string{
	"copy-dependencies",
	"extract-dependencies",
	"load-container-images",
	"sops",
	"docker",
	"postgres",
	"ceph",
	"kubernetes",
	"set-up-cluster",
	"codesphere",
	"ms-backends",
}

// infraSteps are run in Phase 1 (before ArgoCD) when --argocd is active.
var infraSteps = []string{
	"copy-dependencies",
	"extract-dependencies",
	"load-container-images",
	"sops",
	"docker",
	"postgres",
	"ceph",
	"kubernetes",
}

// clusterSteps depend on ArgoCD-managed components and run in Phase 2.
var clusterSteps = []string{"set-up-cluster", "codesphere", "ms-backends"}

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
	Config           string
	Vault            string
	PrivKey          string
	SkipSteps        []string
	CodesphereOnly   bool
	DirectConnection bool
	UseArgoCD        bool
	RegistryURL      string
	VaultFile        string
	AgeKeyPath       string
	VaultNamespace   string
	VaultSecretName  string
}

func (c *InstallCodesphereCmd) RunE(_ *cobra.Command, args []string) error {
	workdir := c.Env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, c.Opts.Package)
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	err := c.ExtractAndInstall(pm, cm, im, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("failed to extract and install package: %w", err)
	}

	return nil
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
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from")
	codesphere.cmd.Flags().BoolVarP(&codesphere.Opts.Force, "force", "f", false, "Enforce package extraction")
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.Config, "config", "c", "", "Path to the Codesphere Private Cloud configuration file (yaml)")
	codesphere.cmd.Flags().StringVar(&codesphere.Opts.Vault, "vault", "prod.vault.yaml", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	codesphere.cmd.Flags().StringSliceVarP(&codesphere.Opts.SkipSteps, "skip-steps", "s", []string{}, "Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, kubernetes")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.CodesphereOnly, "codesphere-only", false, "Install only Codesphere without dependencies")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.UseArgoCD, "argocd", false, "After installation: deploy vault secrets, update the ArgoCD OCI pull secret, and install pc-apps from the BOM version")
	codesphere.cmd.Flags().StringVar(&codesphere.Opts.RegistryURL, "registry-url", "ghcr.io/codesphere-cloud/charts", "OCI registry URL used for the ArgoCD helm pull secret (only relevant with --argocd)")
	codesphere.cmd.Flags().StringVar(&codesphere.Opts.VaultFile, "vault-file", "", "Path to the SOPS-encrypted vault file to deploy as a Kubernetes secret (only relevant with --argocd)")
	codesphere.cmd.Flags().StringVar(&codesphere.Opts.AgeKeyPath, "age-key", "", "Path to the age private key used to decrypt --vault-file (optional, uses default search paths if omitted)")
	codesphere.cmd.Flags().StringVar(&codesphere.Opts.VaultNamespace, "vault-namespace", argocd.DefaultVaultNamespace, "Kubernetes namespace for the vault secret (only relevant with --argocd)")
	codesphere.cmd.Flags().StringVar(&codesphere.Opts.VaultSecretName, "vault-secret-name", argocd.DefaultVaultSecretName, "Name of the Kubernetes secret created from the vault (only relevant with --argocd)")

	util.MarkFlagRequired(codesphere.cmd, "package")
	util.MarkFlagRequired(codesphere.cmd, "config")
	util.MarkFlagRequired(codesphere.cmd, "priv-key")

	AddCmd(install, codesphere.cmd)

	codesphere.cmd.RunE = codesphere.RunE
}

func (c *InstallCodesphereCmd) ExtractAndInstall(pm installer.PackageManager, cm installer.ConfigManager, im system.ImageManager, goos string, goarch string) error {
	if goos != "linux" || goarch != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", goos, goarch)
	}

	originalConfig := c.Opts.Config
	cleanup := func() {}
	if c.Opts.Vault != "" {
		store := installer.NewLazyVaultTemplatingSecretStore(c.Opts.Vault, c.Opts.PrivKey)
		renderedConfig, renderCleanup, err := configtemplating.RenderConfigFileToTemp(c.Opts.Config, store)
		if err != nil {
			return fmt.Errorf("failed to render config template: %w", err)
		}
		cleanup = renderCleanup
		c.Opts.Config = renderedConfig
	}
	defer cleanup()
	defer func() {
		c.Opts.Config = originalConfig
	}()

	config, err := cm.ParseConfigYaml(c.Opts.Config)
	if err != nil {
		return fmt.Errorf("failed to extract config.yaml: %w", err)
	}
	c.warnIfVaultDirDiffersFromSecretsDir(config)

	err = pm.Extract(c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	foundFiles, err := c.ListPackageContents(pm)
	if err != nil {
		return fmt.Errorf("failed to list available files: %w", err)
	}

	if !slices.Contains(foundFiles, "deps.tar.gz") {
		return fmt.Errorf("deps.tar.gz not found in package")
	}
	if !slices.Contains(foundFiles, "private-cloud-installer.js") {
		return fmt.Errorf("private-cloud-installer.js not found in package")
	}
	if !slices.Contains(foundFiles, "node") {
		return fmt.Errorf("node executable not found in package")
	}

	err = pm.ExtractDependency("bom.json", c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	// If workspace image is extended extract bom.json and load workspace image
	for imageKey, imageConfig := range config.Codesphere.DeployConfig.Images {
		for flavorKey, flavor := range imageConfig.Flavors {
			if flavor.Image.Dockerfile != "" && config.Registry != nil && config.Registry.Server != "" {
				bomRef := flavor.Image.BomRef
				dockerfile := flavor.Image.Dockerfile

				fullImageTag, err := pm.GetFullImageTag(bomRef)
				if err != nil {
					return fmt.Errorf("failed to get full image tag for %s: %w", bomRef, err)
				}

				// Extract root image name from full tag (e.g. repo/image:tag -> image)
				parts := strings.Split(fullImageTag, ":")
				if len(parts) < 2 {
					return fmt.Errorf("invalid image tag format: %s", fullImageTag)
				}
				imageNameAndPath := parts[0]
				version := parts[1]
				rootImageName := path.Base(imageNameAndPath)

				// Extract and load root image
				imagePath := filepath.Join("codesphere", "images", fmt.Sprintf("%s.tar", rootImageName))
				err = pm.ExtractDependency(imagePath, c.Opts.Force)
				if err != nil {
					return fmt.Errorf("failed to extract root image %s: %w", imagePath, err)
				}

				extractedImagePath := pm.GetDependencyPath(imagePath)
				err = im.LoadImage(extractedImagePath)
				if err != nil {
					return fmt.Errorf("failed to load workspace image from Dockerfile %s: %w", dockerfile, err)
				}
				log.Printf("Loaded root image '%s'", extractedImagePath)

				// TODO: This is duplicated from update_dockerfile.go, refactor into shared function
				dockerfileFile, err := pm.FileIO().Open(dockerfile)
				if err != nil {
					return fmt.Errorf("failed to open dockerfile %s: %w", dockerfile, err)
				}
				defer util.CloseFileIgnoreError(dockerfileFile)

				dockerfileManager := util.NewDockerfileManager()
				updatedContent, err := dockerfileManager.UpdateFromStatement(dockerfileFile, fullImageTag)
				if err != nil {
					return fmt.Errorf("failed to update FROM statement: %w", err)
				}

				err = pm.FileIO().WriteFile(dockerfile, []byte(updatedContent), 0644)
				if err != nil {
					return fmt.Errorf("failed to write updated dockerfile: %w", err)
				}

				log.Printf("Successfully updated FROM statement in %s to use %s", dockerfile, fullImageTag)
				// TODO: End duplicated code

				dockerfileName := filepath.Base(dockerfile)
				dockerfileDir := filepath.Dir(dockerfile)

				// Determine image tag for build and push
				registryUrl := strings.TrimRight(config.Registry.Server, "/")
				buildTag := fmt.Sprintf("%s/%s-%s:%s", registryUrl, imageKey, flavorKey, version)

				err = im.BuildImage(dockerfileName, buildTag, dockerfileDir)
				if err != nil {
					return fmt.Errorf("failed to build workspace image from Dockerfile %s: %w", dockerfile, err)
				}

				log.Printf("Pushing image to %s", buildTag)
				err = im.PushImage(buildTag)
				if err != nil {
					return fmt.Errorf("failed to push image %s: %w", buildTag, err)
				}
			}
		}
	}

	if err := validateSkipSteps(c.Opts.SkipSteps); err != nil {
		return err
	}

	// Install codesphere with node
	nodePath := filepath.Join(pm.GetWorkDir(), "node")
	err = os.Chmod(nodePath, 0755)
	if err != nil {
		return fmt.Errorf("failed to make node executable: %w", err)
	}

	log.Printf("Using Node.js executable: %s", nodePath)
	installerPath := filepath.Join(pm.GetWorkDir(), "private-cloud-installer.js")
	archivePath := filepath.Join(pm.GetWorkDir(), "deps.tar.gz")

	if c.Opts.UseArgoCD {
		// Phase 1: infra only — skip cluster steps so ArgoCD deps are in place first.
		log.Println("Phase 1: running infra installation (skipping cluster steps)...")
		phase1Args := c.buildInstallerCmdArgs(installerPath, archivePath, clusterSteps)
		if err := c.runInstallerCmd(nodePath, phase1Args); err != nil {
			return fmt.Errorf("phase 1 (infra) installer failed: %w", err)
		}
		log.Println("Phase 1 complete.")

		// ArgoCD integration: deploy secrets and ArgoCD-managed components.
		log.Println("Running ArgoCD integration...")
		if err := c.runArgoCDIntegration(pm, config); err != nil {
			return fmt.Errorf("argocd integration failed: %w", err)
		}
		log.Println("ArgoCD integration complete.")

		// Phase 2: cluster steps — infra is done and ArgoCD components are healthy.
		log.Println("Phase 2: running cluster installation (set-up-cluster, codesphere, ms-backends)...")
		phase2Args := c.buildInstallerCmdArgs(installerPath, archivePath, infraSteps)
		if err := c.runInstallerCmd(nodePath, phase2Args); err != nil {
			return fmt.Errorf("phase 2 (cluster) installer failed: %w", err)
		}
		log.Println("Phase 2 complete.")
	} else {
		log.Println("Starting private cloud installer script...")
		cmdArgs := c.buildInstallerCmdArgs(installerPath, archivePath, nil)
		if err := c.runInstallerCmd(nodePath, cmdArgs); err != nil {
			return fmt.Errorf("failed to run installer script: %w", err)
		}
		log.Println("Private cloud installer script finished.")
	}

	return nil
}

// validateSkipSteps returns an error if any step is not in knownInstallerSteps.
func validateSkipSteps(steps []string) error {
	for _, step := range steps {
		if !slices.Contains(knownInstallerSteps, step) {
			return fmt.Errorf("unknown --skip-step %q; valid steps are: %s", step, strings.Join(knownInstallerSteps, ", "))
		}
	}
	return nil
}

// buildInstallerCmdArgs builds the node command arguments, merging user-provided
// skip steps with any additional steps to skip (e.g. phase-specific steps).
func (c *InstallCodesphereCmd) buildInstallerCmdArgs(installerPath, archivePath string, extraSkips []string) []string {
	skipSet := make(map[string]struct{}, len(c.Opts.SkipSteps)+len(extraSkips))
	for _, s := range c.Opts.SkipSteps {
		skipSet[s] = struct{}{}
	}
	for _, s := range extraSkips {
		skipSet[s] = struct{}{}
	}

	cmdArgs := []string{installerPath, "--archive", archivePath, "--config", c.Opts.Config, "--privKey", c.Opts.PrivKey}
	for step := range skipSet {
		cmdArgs = append(cmdArgs, "--skipStep", step)
	}
	if c.Opts.CodesphereOnly {
		cmdArgs = append(cmdArgs, "--codesphereOnly")
	}
	if c.Opts.DirectConnection {
		cmdArgs = append(cmdArgs, "--directConnection")
	}
	return cmdArgs
}

// runInstallerCmd executes the node installer with the given arguments,
// wiring stdio through to the current process.
func (c *InstallCodesphereCmd) runInstallerCmd(nodePath string, cmdArgs []string) error {
	cmd := exec.Command(nodePath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runArgoCDIntegration deploys vault secrets, updates the ArgoCD OCI pull
// secret, and installs pc-apps using the version recorded in the package BOM.
func (c *InstallCodesphereCmd) runArgoCDIntegration(pm installer.PackageManager, config files.RootConfig) error {
	ctx := context.Background()

	// Build a k8s client with the full client-go scheme so corev1 types are
	// registered (required for vault secret creation).
	kubeConfig, err := ctrlconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubernetes config: %w", err)
	}
	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to register kubernetes scheme: %w", err)
	}
	kubeClient, err := ctrlclient.New(kubeConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Resolve OCI password from env var or interactive prompt.
	ociPassword, err := resolveOCIPassword()
	if err != nil {
		return fmt.Errorf("OCI password required for ArgoCD integration: %w", err)
	}

	// Derive registry URL: prefer explicit flag, then config registry server.
	registryURL := c.Opts.RegistryURL
	if registryURL == "" && config.Registry != nil && config.Registry.Server != "" {
		registryURL = config.Registry.Server
	}

	return argocd.Run(ctx, kubeClient, argocd.Opts{
		BomPath:         pm.GetDependencyPath("bom.json"),
		DatacenterID:    fmt.Sprintf("%d", config.Datacenter.ID),
		OCIPassword:     ociPassword,
		RegistryURL:     registryURL,
		InstallArgoCD:   false,
		VaultFile:       c.Opts.VaultFile,
		AgeKeyPath:      c.Opts.AgeKeyPath,
		VaultNamespace:  c.Opts.VaultNamespace,
		VaultSecretName: c.Opts.VaultSecretName,
	})
}

func (c *InstallCodesphereCmd) warnIfVaultDirDiffersFromSecretsDir(config files.RootConfig) {
	if c.Opts.Vault == "" || config.Secrets.BaseDir == "" {
		return
	}

	vaultDir, err := filepath.Abs(filepath.Dir(c.Opts.Vault))
	if err != nil {
		log.Printf("Warning: failed to resolve vault directory for %s: %v", c.Opts.Vault, err)
		return
	}

	secretsDir, err := filepath.Abs(config.Secrets.BaseDir)
	if err != nil {
		log.Printf("Warning: failed to resolve configured secrets baseDir %s: %v", config.Secrets.BaseDir, err)
		return
	}

	if vaultDir != secretsDir {
		log.Printf("Warning: config secrets.baseDir (%s) does not match the directory of --vault (%s)", secretsDir, vaultDir)
	}
}

func (c *InstallCodesphereCmd) ListPackageContents(pm installer.PackageManager) ([]string, error) {
	packageDir := pm.GetWorkDir()
	if !pm.FileIO().Exists(packageDir) {
		return nil, fmt.Errorf("work dir not found: %s", packageDir)
	}

	entries, err := pm.FileIO().ReadDir(packageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory contents: %w", err)
	}

	log.Printf("Listing contents of %s", packageDir)
	var foundFiles []string
	for _, entry := range entries {
		filename := entry.Name()
		log.Printf("- %s", filename)
		foundFiles = append(foundFiles, filename)
	}

	return foundFiles, nil
}
