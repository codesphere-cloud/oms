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

	originalConfig := ci.ConfigPath
	cleanup := func() {}
	if ci.VaultPath != "" {
		store := NewLazyVaultTemplatingSecretStore(ci.VaultPath, ci.PrivKey)
		renderedConfig, renderCleanup, err := configtemplating.RenderConfigFileToTemp(ci.ConfigPath, store)
		if err != nil {
			return fmt.Errorf("failed to render config template: %w", err)
		}
		cleanup = renderCleanup
		ci.ConfigPath = renderedConfig
	}
	defer cleanup()
	defer func() {
		ci.ConfigPath = originalConfig
	}()

	config, err := cm.ParseConfigYaml(ci.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to extract config.yaml: %w", err)
	}
	ci.warnIfVaultDirDiffersFromSecretsDir(config)

	err = pm.Extract(ci.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	foundFiles, err := ListPackageContents(pm)
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

	err = pm.ExtractDependency("bom.json", ci.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	// If workspace image is extended extract bom.json and load workspace image
	if !ci.SkipImageBuilding {
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
					err = pm.ExtractDependency(imagePath, ci.Force)
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
	}

	// Install codesphere with node
	nodePath := filepath.Join(pm.GetWorkDir(), "node")
	err = os.Chmod(nodePath, 0755)
	if err != nil {
		return fmt.Errorf("failed to make node executable: %w", err)
	}

	log.Printf("Using Node.js executable: %s", nodePath)
	log.Println("Starting private cloud installer script...")
	installerPath := filepath.Join(pm.GetWorkDir(), "private-cloud-installer.js")
	archivePath := filepath.Join(pm.GetWorkDir(), "deps.tar.gz")

	cmdArgs := []string{installerPath, "--archive", archivePath, "--config", ci.ConfigPath, "--privKey", ci.PrivKey}

	baseSteps := KnownInstallerSteps
	if len(ci.AllowedSteps) > 0 {
		baseSteps = ci.AllowedSteps
	}

	executableSteps := map[string]bool{}
	for _, step := range baseSteps {
		executableSteps[step] = true
	}

	// Steps not in baseSteps are skipped implicitly; add them to cmdArgs
	for _, step := range KnownInstallerSteps {
		if _, ok := executableSteps[step]; !ok {
			cmdArgs = append(cmdArgs, "--skipStep", step)
		}
	}

	if config.Operations != nil {
		for _, step := range config.Operations.Skip {
			executableSteps[step] = false
		}
	}

	for _, step := range ci.SkipSteps {
		executableSteps[step] = false
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
		return fmt.Errorf("installation aborted")
	}

	if ci.CodesphereOnly {
		cmdArgs = append(cmdArgs, "--codesphereOnly")
	}

	if ci.DirectConnection {
		cmdArgs = append(cmdArgs, "--directConnection")
	}

	cmd := exec.Command(nodePath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run installer script: %w", err)
	}
	log.Println("Private cloud installer script finished.")

	return nil
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

// ListPackageContents returns the filenames present in the extracted package work directory.
func ListPackageContents(pm PackageManager) ([]string, error) {
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
