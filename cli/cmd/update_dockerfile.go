package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type UpdateDockerfileCmd struct {
	cmd  *cobra.Command
	Opts UpdateDockerfileOpts
	Env  env.Env
}

type UpdateDockerfileOpts struct {
	*GlobalOptions
	Package    string
	Dockerfile string
	Baseimage  string
	Force      bool
}

func (c *UpdateDockerfileCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.Package == "" {
		return errors.New("required option package not set")
	}

	workdir := c.Env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, c.Opts.Package)
	im := system.NewImage(context.Background())

	err := c.UpdateDockerfile(pm, im, args)
	if err != nil {
		return fmt.Errorf("failed to update dockerfile: %w", err)
	}

	return nil
}

func AddUpdateDockerfileCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	dockerfileCmd := &UpdateDockerfileCmd{
		cmd: &cobra.Command{
			Use:   "dockerfile",
			Short: "Update FROM statement in Dockerfile with base image from package",
			Long: `Update the FROM statement in a Dockerfile to use the base image from a Codesphere package.

This command extracts the base image from a Codesphere package and updates the FROM statement
in the specified Dockerfile to use that base image. The base image is loaded into the local Docker daemon so it can be used for building.`,
			Example: formatExamplesWithBinary("update dockerfile", []io.Example{
				{Cmd: "--dockerfile baseimage/Dockerfile --package codesphere-v1.68.0.tar.gz", Desc: "Update Dockerfile to use the default base image from the package (workspace-agent-24.04)"},
				{Cmd: "--dockerfile baseimage/Dockerfile --package codesphere-v1.68.0.tar.gz --baseimage workspace-agent-20.04.tar", Desc: "Update Dockerfile to use the workspace-agent-20.04 base image from the package"},
			}, "oms-cli"),
			Args: cobra.ExactArgs(0),
		},
		Opts: UpdateDockerfileOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}

	dockerfileCmd.cmd.Flags().StringVarP(&dockerfileCmd.Opts.Dockerfile, "dockerfile", "d", "", "Path to the Dockerfile to update (required)")
	dockerfileCmd.cmd.Flags().StringVarP(&dockerfileCmd.Opts.Package, "package", "p", "", "Path to the Codesphere package (required)")
	dockerfileCmd.cmd.Flags().StringVarP(&dockerfileCmd.Opts.Baseimage, "baseimage", "b", "workspace-agent-24.04", "Name of the base image to use (required)")
	dockerfileCmd.cmd.Flags().BoolVarP(&dockerfileCmd.Opts.Force, "force", "f", false, "Force re-extraction of the package")

	util.MarkFlagRequired(dockerfileCmd.cmd, "dockerfile")
	util.MarkFlagRequired(dockerfileCmd.cmd, "package")

	parentCmd.AddCommand(dockerfileCmd.cmd)

	dockerfileCmd.cmd.RunE = dockerfileCmd.RunE
}

func (c *UpdateDockerfileCmd) UpdateDockerfile(pm installer.PackageManager, im system.ImageManager, args []string) error {
	imageName, err := pm.GetBaseimageName(c.Opts.Baseimage)
	if err != nil {
		return fmt.Errorf("failed to get image name: %w", err)
	}

	imagePath, err := pm.GetBaseimagePath(c.Opts.Baseimage, c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to get image path: %w", err)
	}

	log.Printf("Loading container image from package into local docker daemon: %s", imagePath)

	// Loading image before updating the Dockerfile to ensure it's available for builds
	err = im.LoadImage(imagePath)
	if err != nil {
		return fmt.Errorf("failed to load baseimage file %s: %w", imagePath, err)
	}

	// Update dockerfile FROM statement
	dockerfileFile, err := pm.FileIO().Open(c.Opts.Dockerfile)
	if err != nil {
		return fmt.Errorf("failed to open dockerfile %s: %w", c.Opts.Dockerfile, err)
	}
	defer util.CloseFileIgnoreError(dockerfileFile)

	dockerfileManager := util.NewDockerfileManager()
	updatedContent, err := dockerfileManager.UpdateFromStatement(dockerfileFile, imageName)
	if err != nil {
		return fmt.Errorf("failed to update FROM statement: %w", err)
	}

	err = pm.FileIO().WriteFile(c.Opts.Dockerfile, []byte(updatedContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated dockerfile: %w", err)
	}

	log.Printf("Successfully updated FROM statement in %s to use %s", c.Opts.Dockerfile, imageName)

	return nil
}
