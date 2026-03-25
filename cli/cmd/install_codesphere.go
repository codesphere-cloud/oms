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
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// InstallCodesphereCmd represents the codesphere command
type InstallCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

type InstallCodesphereOpts struct {
	*GlobalOptions
	Package   string
	Force     bool
	Config    string
	PrivKey   string
	SkipSteps []string
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
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	codesphere.cmd.Flags().StringSliceVarP(&codesphere.Opts.SkipSteps, "skip-steps", "s", []string{}, "Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, kubernetes")

	util.MarkFlagRequired(codesphere.cmd, "package")
	util.MarkFlagRequired(codesphere.cmd, "config")
	util.MarkFlagRequired(codesphere.cmd, "priv-key")

	install.AddCommand(codesphere.cmd)

	codesphere.cmd.RunE = codesphere.RunE
}

func (c *InstallCodesphereCmd) ExtractAndInstall(pm installer.PackageManager, cm installer.ConfigManager, im system.ImageManager, goos string, goarch string) error {
	if goos != "linux" || goarch != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", goos, goarch)
	}

	config, err := cm.ParseConfigYaml(c.Opts.Config)
	if err != nil {
		return fmt.Errorf("failed to extract config.yaml: %w", err)
	}

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
	for _, imageConfig := range config.Codesphere.DeployConfig.Images {
		for _, flavor := range imageConfig.Flavors {
			if flavor.Image.Dockerfile != "" && config.Registry != nil && config.Registry.Server != "" {
				bomRef := flavor.Image.BomRef
				dockerfile := flavor.Image.Dockerfile

				fullImageTag, err := pm.GetFullImageTag(bomRef)
				if err != nil {
					return fmt.Errorf("failed to get full image tag for %s: %w", bomRef, err)
				}

				// Extract root image name from full tag (e.g. repo/image:tag -> image)
				parts := strings.Split(fullImageTag, ":")
				if len(parts) == 0 {
					return fmt.Errorf("invalid image tag format: %s", fullImageTag)
				}
				imageNameAndPath := parts[0]
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
				buildTag := fmt.Sprintf("%s/codesphere-registry/%s-%s", registryUrl, config.Codesphere.DeployConfig.Images)

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

	cmdArgs := []string{installerPath, "--archive", archivePath, "--config", c.Opts.Config, "--privKey", c.Opts.PrivKey}
	if len(c.Opts.SkipSteps) > 0 {
		for _, step := range c.Opts.SkipSteps {
			cmdArgs = append(cmdArgs, "--skipStep", step)
		}
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
