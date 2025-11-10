// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/tmpl"
)

// ExtendBaseimageCmd represents the baseimage command
type ExtendBaseimageCmd struct {
	cmd  *cobra.Command
	Opts *ExtendBaseimageOpts
	Env  env.Env
}

type ExtendBaseimageOpts struct {
	*GlobalOptions
	Package    string
	Dockerfile string
	Baseimage  string
	Force      bool
}

func (c *ExtendBaseimageCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.Package == "" {
		return errors.New("required option package not set")
	}

	workdir := c.Env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, c.Opts.Package)
	im := system.NewImage(context.Background())

	err := c.ExtendBaseimage(pm, im)
	if err != nil {
		return fmt.Errorf("failed to extend baseimage: %w", err)
	}

	return nil
}

func AddExtendBaseimageCmd(extend *cobra.Command, opts *GlobalOptions) {
	baseimage := ExtendBaseimageCmd{
		cmd: &cobra.Command{
			Use:   "baseimage",
			Short: "Extend Codesphere's workspace base image for customization",
			Long: io.Long(`Loads the baseimage from Codesphere package and generates a Dockerfile based on it.
				This enables you to extend Codesphere's base image with specific dependencies.

				To use the custom base image, you need to push the resulting image to your container registry and
				reference it in your install-config for the Codesphere installation process to pick it up and include it in Codesphere`),
		},
		Opts: &ExtendBaseimageOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	baseimage.cmd.Flags().StringVarP(&baseimage.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load base image from")
	baseimage.cmd.Flags().StringVarP(&baseimage.Opts.Dockerfile, "dockerfile", "d", "Dockerfile", "Output Dockerfile to generate for extending the base image")
	baseimage.cmd.Flags().StringVarP(&baseimage.Opts.Baseimage, "baseimage", "b", "workspace-agent-24.04", "Base image file name inside the package to extend (default: 'workspace-agent-24.04')")
	baseimage.cmd.Flags().BoolVarP(&baseimage.Opts.Force, "force", "f", false, "Enforce package extraction")

	extend.AddCommand(baseimage.cmd)

	baseimage.cmd.RunE = baseimage.RunE
}

func (c *ExtendBaseimageCmd) ExtendBaseimage(pm installer.PackageManager, im system.ImageManager) error {
	err := pm.Extract(c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}

	imageName, err := pm.GetBaseimageName(c.Opts.Baseimage)
	if err != nil {
		return fmt.Errorf("failed to get image name: %w", err)
	}

	imagePath, err := pm.GetBaseimagePath(c.Opts.Baseimage, c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to get image path: %w", err)
	}

	err = tmpl.GenerateDockerfile(pm.FileIO(), c.Opts.Dockerfile, imageName)
	if err != nil {
		return fmt.Errorf("failed to generate dockerfile: %w", err)
	}

	log.Printf("Loading container image from package into local docker daemon: %s", imagePath)

	err = im.LoadImage(imagePath)
	if err != nil {
		return fmt.Errorf("failed to load baseimage file %s: %w", imagePath, err)
	}

	return nil
}
