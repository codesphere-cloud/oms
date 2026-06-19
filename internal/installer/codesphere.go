// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
)

// KnownInstallerSteps mirrors SKIPPABLE_STEPS from private-cloud-installer.ts.
var KnownInstallerSteps = []string{
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

// ArgoCDStep is the skip-step name for the dependency-phase ArgoCD pre-step.
const ArgoCDStep = "argocd"

// InfraSteps are run in Phase 1 (before ArgoCD) when --argocd is active.
var InfraSteps = []string{
	"copy-dependencies",
	"extract-dependencies",
	"load-container-images",
	"sops",
	"docker",
	"postgres",
	"ceph",
	"kubernetes",
}

// DependenciesSteps run in Phase 2 after infrastructure is ready.
var DependenciesSteps = []string{"set-up-cluster", "ms-backends"}

// PlatformSteps run in Phase 3 and install the Codesphere platform only.
var PlatformSteps = []string{"codesphere"}

// CodesphereInstaller encapsulates the logic for running the private-cloud-installer.js script.
type CodesphereInstaller struct {
	// ConfigPath is the path to the Codesphere Private Cloud configuration YAML file.
	ConfigPath string
	// VaultPath is the path to the SOPS-encrypted vault file used for config templating.
	// When non-empty, the config is rendered as a template before use.
	VaultPath string
	// PrivKey is the path to the age/GPG private key used to decrypt the vault.
	PrivKey string
	// Force re-extracts the package even if the work directory already exists.
	Force bool
	// SkipSteps lists installer steps to skip within the active step set.
	SkipSteps []string
	// AllowedSteps restricts execution to a subset of KnownInstallerSteps.
	// When nil, all KnownInstallerSteps are used. Set to InfraSteps, DependenciesSteps,
	// or PlatformSteps for the split-phase commands.
	AllowedSteps []string
	// CodesphereOnly skips all dependency steps and only runs the Codesphere helm charts.
	CodesphereOnly bool
	// DirectConnection routes installer traffic directly to cluster nodes instead of
	// going through a bastion; requires network access to those nodes from the local machine.
	DirectConnection bool
	// AutoApprove suppresses the confirmation prompt before executing steps.
	AutoApprove bool
	// SkipImageBuilding skips the custom workspace image build-and-push step.
	// Set when a prior phase already built the images so they are not rebuilt redundantly.
	SkipImageBuilding bool
}

// Install validates the platform, extracts the package, builds any custom workspace images,
// and runs the private-cloud-installer.js script.
func (ci *CodesphereInstaller) Install(pm PackageManager, cm ConfigManager, im system.ImageManager, goos, goarch string) error {
	if goos != "linux" || goarch != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", goos, goarch)
	}

	config, cleanup, err := ci.prepareConfig(cm)
	defer cleanup()
	if err != nil {
		return err
	}
	if !ci.HasExecutableSteps(config) {
		log.Println("No executable installer steps remain after applying skip configuration. Skipping installer run.")
		return nil
	}

	if err := ci.ExtractAndValidatePackage(pm); err != nil {
		return err
	}

	if err := ci.buildWorkspaceImages(pm, im, config); err != nil {
		return err
	}

	return ci.runInstaller(pm, config)
}

func (ci *CodesphereInstaller) prepareConfig(cm ConfigManager) (files.RootConfig, func(), error) {
	originalConfig := ci.ConfigPath
	cleanup := func() {
		ci.ConfigPath = originalConfig
	}

	if ci.VaultPath != "" {
		store := NewLazyVaultTemplatingSecretStore(ci.VaultPath, ci.PrivKey)
		renderedConfig, renderCleanup, err := configtemplating.RenderConfigFileToTemp(ci.ConfigPath, store)
		if err != nil {
			return files.RootConfig{}, cleanup, fmt.Errorf("failed to render config template: %w", err)
		}

		cleanup = func() {
			ci.ConfigPath = originalConfig
			renderCleanup()
		}
		ci.ConfigPath = renderedConfig
	}

	config, err := cm.ParseConfigYaml(ci.ConfigPath)
	if err != nil {
		return files.RootConfig{}, cleanup, fmt.Errorf("failed to extract config.yaml: %w", err)
	}

	ci.warnIfVaultDirDiffersFromSecretsDir(config)
	return config, cleanup, nil
}

// IsStepSkipped reports whether step is present in persisted or CLI skip steps.
func IsStepSkipped(config files.RootConfig, skipSteps []string, step string) bool {
	skippedSteps := map[string]bool{}
	if config.Operations != nil {
		for _, skippedStep := range config.Operations.Skip {
			skippedSteps[skippedStep] = true
		}
	}
	for _, skippedStep := range skipSteps {
		skippedSteps[skippedStep] = true
	}
	return skippedSteps[step]
}

// ApplySkippedSteps marks known executable steps as skipped from persisted or CLI skip steps.
func ApplySkippedSteps(executableSteps map[string]bool, config files.RootConfig, skipSteps []string) {
	if config.Operations != nil {
		for _, step := range config.Operations.Skip {
			if _, ok := executableSteps[step]; ok {
				executableSteps[step] = false
			}
		}
	}

	for _, step := range skipSteps {
		if _, ok := executableSteps[step]; ok {
			executableSteps[step] = false
		}
	}
}

func (ci *CodesphereInstaller) warnIfVaultDirDiffersFromSecretsDir(config files.RootConfig) {
	if ci.VaultPath == "" || config.Secrets.BaseDir == "" {
		return
	}

	vaultDir, err := filepath.Abs(filepath.Dir(ci.VaultPath))
	if err != nil {
		log.Printf("Warning: failed to resolve vault directory for %s: %v", ci.VaultPath, err)
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

func (ci *CodesphereInstaller) ExtractAndValidatePackage(pm PackageManager) error {
	if err := pm.Extract(ci.Force); err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	foundFiles, err := ci.listPackageFiles(pm)
	if err != nil {
		return err
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

	if err := pm.ExtractDependency("bom.json", ci.Force); err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	return nil
}

func (ci *CodesphereInstaller) listPackageFiles(pm PackageManager) ([]string, error) {
	packageDir := pm.GetWorkDir()
	if !pm.FileIO().Exists(packageDir) {
		return nil, fmt.Errorf("failed to list available files: work dir not found: %s", packageDir)
	}

	entries, err := pm.FileIO().ReadDir(packageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list available files: failed to read directory contents: %w", err)
	}

	log.Printf("Listing contents of %s", packageDir)
	foundFiles := []string{}
	for _, entry := range entries {
		filename := entry.Name()
		log.Printf("- %s", filename)
		foundFiles = append(foundFiles, filename)
	}

	return foundFiles, nil
}

func (ci *CodesphereInstaller) buildWorkspaceImages(pm PackageManager, im system.ImageManager, config files.RootConfig) error {
	if ci.SkipImageBuilding {
		return nil
	}

	for imageKey, imageConfig := range config.Codesphere.DeployConfig.Images {
		for flavorKey, flavor := range imageConfig.Flavors {
			if !shouldBuildWorkspaceImage(config, flavor) {
				continue
			}

			if err := ci.buildWorkspaceImage(pm, im, config, imageKey, flavorKey, flavor); err != nil {
				return err
			}
		}
	}

	return nil
}

func shouldBuildWorkspaceImage(config files.RootConfig, flavor files.FlavorConfig) bool {
	return flavor.Image.Dockerfile != "" && config.Registry != nil && config.Registry.Server != ""
}

func (ci *CodesphereInstaller) buildWorkspaceImage(
	pm PackageManager,
	im system.ImageManager,
	config files.RootConfig,
	imageKey string,
	flavorKey string,
	flavor files.FlavorConfig,
) error {
	bomRef := flavor.Image.BomRef
	dockerfile := flavor.Image.Dockerfile

	fullImageTag, err := pm.GetFullImageTag(bomRef)
	if err != nil {
		return fmt.Errorf("failed to get full image tag for %s: %w", bomRef, err)
	}

	rootImageName, version, err := splitImageTag(fullImageTag)
	if err != nil {
		return err
	}

	if err := ci.extractAndLoadRootImage(pm, im, rootImageName, dockerfile); err != nil {
		return err
	}

	if err := updateDockerfileFromStatement(pm, dockerfile, fullImageTag); err != nil {
		return err
	}

	buildTag := workspaceImageBuildTag(config.Registry.Server, imageKey, flavorKey, version)
	dockerfileName := filepath.Base(dockerfile)
	dockerfileDir := filepath.Dir(dockerfile)

	if err := im.BuildImage(dockerfileName, buildTag, dockerfileDir); err != nil {
		return fmt.Errorf("failed to build workspace image from Dockerfile %s: %w", dockerfile, err)
	}

	log.Printf("Pushing image to %s", buildTag)
	if err := im.PushImage(buildTag); err != nil {
		return fmt.Errorf("failed to push image %s: %w", buildTag, err)
	}

	return nil
}

func splitImageTag(fullImageTag string) (string, string, error) {
	parts := strings.Split(fullImageTag, ":")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid image tag format: %s", fullImageTag)
	}

	imageNameAndPath := parts[0]
	version := parts[1]
	return path.Base(imageNameAndPath), version, nil
}

func (ci *CodesphereInstaller) extractAndLoadRootImage(pm PackageManager, im system.ImageManager, rootImageName, dockerfile string) error {
	imagePath := filepath.Join("codesphere", "images", fmt.Sprintf("%s.tar", rootImageName))
	if err := pm.ExtractDependency(imagePath, ci.Force); err != nil {
		return fmt.Errorf("failed to extract root image %s: %w", imagePath, err)
	}

	extractedImagePath := pm.GetDependencyPath(imagePath)
	if err := im.LoadImage(extractedImagePath); err != nil {
		return fmt.Errorf("failed to load workspace image from Dockerfile %s: %w", dockerfile, err)
	}

	log.Printf("Loaded root image '%s'", extractedImagePath)
	return nil
}

func updateDockerfileFromStatement(pm PackageManager, dockerfile, fullImageTag string) error {
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

	if err := pm.FileIO().WriteFile(dockerfile, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write updated dockerfile: %w", err)
	}

	log.Printf("Successfully updated FROM statement in %s to use %s", dockerfile, fullImageTag)
	return nil
}

func workspaceImageBuildTag(registryServer, imageKey, flavorKey, version string) string {
	registryURL := strings.TrimRight(registryServer, "/")
	return fmt.Sprintf("%s/%s-%s:%s", registryURL, imageKey, flavorKey, version)
}

func (ci *CodesphereInstaller) runInstaller(pm PackageManager, config files.RootConfig) error {
	nodePath := filepath.Join(pm.GetWorkDir(), "node")
	if err := os.Chmod(nodePath, 0755); err != nil {
		return fmt.Errorf("failed to make node executable: %w", err)
	}

	log.Printf("Using Node.js executable: %s", nodePath)
	log.Println("Starting private cloud installer script...")

	cmdArgs, err := ci.installerCommandArgs(pm, config)
	if err != nil {
		return err
	}

	cmd := exec.Command(nodePath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	log.Printf("Running private cloud installer command: %s", shellQuotedCommand(append([]string{nodePath}, cmdArgs...)...))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run installer script: %w", err)
	}

	log.Println("Private cloud installer script finished.")
	return nil
}

func (ci *CodesphereInstaller) installerCommandArgs(pm PackageManager, config files.RootConfig) ([]string, error) {
	installerPath := filepath.Join(pm.GetWorkDir(), "private-cloud-installer.js")
	archivePath := filepath.Join(pm.GetWorkDir(), "deps.tar.gz")
	cmdArgs := []string{installerPath, "--archive", archivePath, "--config", ci.ConfigPath, "--privKey", ci.PrivKey}

	executableSteps := ci.executableInstallerSteps(config)

	for _, step := range KnownInstallerSteps {
		if _, ok := executableSteps[step]; !ok {
			cmdArgs = append(cmdArgs, "--skipStep", step)
		}
	}

	executedSteps := []string{}
	for step, executed := range executableSteps {
		if !executed {
			cmdArgs = append(cmdArgs, "--skipStep", step)
		} else {
			executedSteps = append(executedSteps, step)
		}
	}

	sort.Strings(executedSteps)

	prompt := NewPrompter(!ci.AutoApprove)
	msg := fmt.Sprintf("The following steps will be executed: %s. Type \"yes\" to continue.", strings.Join(executedSteps, ", "))
	if prompt.String(msg, "yes") != "yes" {
		return nil, fmt.Errorf("installation aborted")
	}

	if ci.CodesphereOnly {
		cmdArgs = append(cmdArgs, "--codesphereOnly")
	}

	if ci.DirectConnection {
		cmdArgs = append(cmdArgs, "--directConnection")
	}

	return cmdArgs, nil
}

func (ci *CodesphereInstaller) executableInstallerSteps(config files.RootConfig) map[string]bool {
	executableSteps := map[string]bool{}
	for _, step := range KnownInstallerSteps {
		executableSteps[step] = len(ci.AllowedSteps) == 0
	}

	if len(ci.AllowedSteps) > 0 {
		for _, step := range ci.AllowedSteps {
			executableSteps[step] = true
		}
	}

	ApplySkippedSteps(executableSteps, config, ci.SkipSteps)

	return executableSteps
}

// ExecutableSteps returns the sorted installer steps that remain after allowlist and skip filtering.
func (ci *CodesphereInstaller) ExecutableSteps(config files.RootConfig) []string {
	executableSteps := ci.executableInstallerSteps(config)
	steps := make([]string, 0, len(executableSteps))
	for _, step := range KnownInstallerSteps {
		if executableSteps[step] {
			steps = append(steps, step)
		}
	}

	return steps
}

// HasExecutableSteps reports whether at least one installer step remains after applying skips.
func (ci *CodesphereInstaller) HasExecutableSteps(config files.RootConfig) bool {
	return len(ci.ExecutableSteps(config)) > 0
}

func shellQuotedCommand(args ...string) string {
	quotedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		quotedArgs = append(quotedArgs, strconv.Quote(arg))
	}

	return strings.Join(quotedArgs, " ")
}
