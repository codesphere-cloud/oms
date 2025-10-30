// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// BuildImagesCmd represents the build images command
type BuildImagesCmd struct {
	cmd  *cobra.Command
	Opts *BuildImagesOpts
	Env  env.Env
}

type BuildImagesOpts struct {
	GlobalOptions
	Config string
}

func (c *BuildImagesCmd) RunE(_ *cobra.Command, args []string) error {
	pm := installer.NewPackage(c.Env.GetOmsWorkdir(), c.Opts.Config)
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	err := c.BuildAndPushImages(pm, cm, im)
	if err != nil {
		return fmt.Errorf("failed to build and push images: %w", err)
	}

	return nil
}

func AddBuildImagesCmd(build *cobra.Command, opts GlobalOptions) {
	buildImages := BuildImagesCmd{
		cmd: &cobra.Command{
			Use:   "images",
			Short: "Build and push container images",
			Long: io.Long(`Build and push container images based on the configuration file.
			Extracts necessary image configurations from the provided install config and the downloaded package.`),
		},
		Opts: &BuildImagesOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	buildImages.cmd.Flags().StringVarP(&buildImages.Opts.Config, "config", "c", "", "Path to the configuration YAML file")

	util.MarkFlagRequired(buildImages.cmd, "config")

	build.AddCommand(buildImages.cmd)

	buildImages.cmd.RunE = buildImages.RunE
}

func (c *BuildImagesCmd) BuildAndPushImages(pm installer.PackageManager, cm installer.ConfigManager, im system.ImageManager) error {
	config, err := cm.ParseConfigYaml(c.Opts.Config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if len(config.Codesphere.DeployConfig.Images) == 0 {
		return fmt.Errorf("no images defined in the config")
	}
	if len(config.Registry.Server) == 0 {
		return fmt.Errorf("registry server not defined in the config")
	}

	codesphereVersion, err := pm.GetCodesphereVersion()
	if err != nil {
		return fmt.Errorf("failed to get codesphere version from package: %w", err)
	}

	for imageName, imageConfig := range config.Codesphere.DeployConfig.Images {
		for flavorName, flavorConfig := range imageConfig.Flavors {
			log.Printf("Processing image '%s' with flavor '%s'", imageName, flavorName)
			if flavorConfig.Image.Dockerfile == "" {
				log.Printf("Skipping flavor '%s', no dockerfile defined", flavorName)
				continue
			}

			targetImage := fmt.Sprintf("%s/%s-%s:%s", config.Registry.Server, imageName, flavorName, codesphereVersion)

			err := im.BuildImage(flavorConfig.Image.Dockerfile, targetImage, ".")
			if err != nil {
				return fmt.Errorf("failed to build image %s: %w", targetImage, err)
			}

			err = im.PushImage(targetImage)
			if err != nil {
				return fmt.Errorf("failed to push image %s: %w", targetImage, err)
			}

			log.Printf("Successfully built and pushed image: %s", targetImage)
		}
	}

	return nil
}
