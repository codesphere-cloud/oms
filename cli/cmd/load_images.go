// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/registry"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
)

const packageBomJSON = "bom.json"

type LoadImagesCmd struct {
	cmd    *cobra.Command
	Opts   *LoadImagesOpts
	Copier registry.ImageCopier
	Env    env.Env
}

type LoadImagesOpts struct {
	*GlobalOptions
	DryRun bool
	Force  bool
}

func AddLoadImagesCmd(load *cobra.Command, opts *GlobalOptions) {
	c := &LoadImagesCmd{
		cmd: &cobra.Command{
			Use:   "images <package> <target-registry>",
			Short: "Mirror all Codesphere OCI images required to install package from Codesphere's registry",
			Long: packageio.Long(`Mirror all Codesphere OCI images required to install package from Codesphere's registry into a target registry.
				This is required for installations that require a custom registry, such as air-gapped environments.

				Ensure that the target registry is reachable and that you have permission to push images to it.
				To use the custom registry, it must be configured before installing Codesphere.`),
			Example: formatExamples("load images", []packageio.Example{
				{
					Cmd:  "codesphere-v1.68.0.tar.gz registry.internal.example.com",
					Desc: "Mirror every Codesphere OCI image reference from the package BOM into the target registry",
				},
			}),
			Args: cobra.ExactArgs(2),
		},
		Opts: &LoadImagesOpts{
			GlobalOptions: opts,
		},
		Env: env.NewEnv(),
	}

	c.cmd.Flags().BoolVar(&c.Opts.DryRun, "dry-run", false, "Print planned copy operations without copying images")
	c.cmd.Flags().BoolVarP(&c.Opts.Force, "force", "f", false, "Force new package extraction even if already extracted")

	AddCmd(load, c.cmd)
	c.cmd.RunE = c.RunE
}

func (c *LoadImagesCmd) RunE(cmd *cobra.Command, args []string) error {
	pm := installer.NewPackage(c.Env.GetOmsWorkdir(), args[0])
	return c.LoadImagesFromPackage(cmd.Context(), pm, args[1])
}

func (c *LoadImagesCmd) LoadImagesFromPackage(ctx context.Context, pm installer.PackageManager, targetRegistry string) error {
	bomPath, err := c.extractPackageBom(pm)
	if err != nil {
		return err
	}

	return c.LoadImagesFromBomPath(ctx, bomPath, targetRegistry)
}

func (c *LoadImagesCmd) LoadImagesFromBomPath(ctx context.Context, bomPath string, targetRegistry string) error {
	config, err := bom.Parse(bomPath)
	if err != nil {
		return err
	}

	copier := c.Copier
	if !c.Opts.DryRun && copier == nil {
		copier = system.NewRegistryImageCopier(ctx)
	}

	mirror := registry.Mirror{
		Copier: copier,
		DryRun: c.Opts.DryRun,
	}
	_, err = mirror.MirrorGHCRImages(config, targetRegistry)
	if err != nil {
		return fmt.Errorf("failed to load images from %s: %w", bomPath, err)
	}

	return nil
}

func (c *LoadImagesCmd) extractPackageBom(pm installer.PackageManager) (string, error) {
	if err := pm.ExtractDependency(packageBomJSON, c.Opts.Force); err != nil {
		return "", fmt.Errorf("failed to extract %s from package: %w", packageBomJSON, err)
	}

	return pm.GetDependencyPath(packageBomJSON), nil
}
