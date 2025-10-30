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

// BuildImageCmd represents the build image command
type BuildImageCmd struct {
	cmd  *cobra.Command
	Opts BuildImageOpts
	Env  env.Env
}

type BuildImageOpts struct {
	GlobalOptions
	Dockerfile string
	Package    string
	Registry   string
}

func (c *BuildImageCmd) RunE(cmd *cobra.Command, args []string) error {
	pm := installer.NewPackage(c.Env.GetOmsWorkdir(), c.Opts.Package)
	im := system.NewImage(context.Background())

	return c.BuildImage(pm, im)
}

func AddBuildImageCmd(parentCmd *cobra.Command, opts GlobalOptions) {
	imageCmd := &BuildImageCmd{
		cmd: &cobra.Command{
			Use:   "image",
			Short: "Build and push Docker image using Dockerfile and Codesphere package version",
			Long:  `Build a Docker image from a Dockerfile and push it to a registry, tagged with the Codesphere version from the package.`,
			Example: formatExamplesWithBinary("build image", []io.Example{
				{Cmd: "--dockerfile baseimage/Dockerfile --package codesphere-v1.68.0.tar.gz --registry my-registry.com/my-image", Desc: "Build image for Codesphere version 1.68.0 and push to specified registry"},
			}, "oms-cli"),
			Args: cobra.ExactArgs(0),
		},
		Opts: BuildImageOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}

	imageCmd.cmd.Flags().StringVarP(&imageCmd.Opts.Dockerfile, "dockerfile", "d", "", "Path to the Dockerfile to build (required)")
	imageCmd.cmd.Flags().StringVarP(&imageCmd.Opts.Package, "package", "p", "", "Path to the Codesphere package (required)")
	imageCmd.cmd.Flags().StringVarP(&imageCmd.Opts.Registry, "registry", "r", "", "Registry URL to push to (e.g., my-registry.com/my-image) (required)")

	util.MarkFlagRequired(imageCmd.cmd, "dockerfile")
	util.MarkFlagRequired(imageCmd.cmd, "package")
	util.MarkFlagRequired(imageCmd.cmd, "registry")

	parentCmd.AddCommand(imageCmd.cmd)

	imageCmd.cmd.RunE = imageCmd.RunE
}

// AddBuildImageCmd adds the build image command to the parent command
func (c *BuildImageCmd) BuildImage(pm installer.PackageManager, im system.ImageManager) error {
	codesphereVersion, err := pm.GetCodesphereVersion()
	if err != nil {
		return fmt.Errorf("failed to get codesphere version from package: %w", err)
	}

	targetImage := fmt.Sprintf("%s:%s", c.Opts.Registry, codesphereVersion)

	err = im.BuildImage(c.Opts.Dockerfile, targetImage, ".")
	if err != nil {
		return fmt.Errorf("failed to build image %s: %w", targetImage, err)
	}

	err = im.PushImage(targetImage)
	if err != nil {
		return fmt.Errorf("failed to push image %s: %w", targetImage, err)
	}

	log.Printf("Successfully built and pushed image: %s", targetImage)

	return nil
}
