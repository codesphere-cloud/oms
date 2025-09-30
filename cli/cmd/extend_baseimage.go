package cmd

import (
	"errors"
	"fmt"
	"path"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
)

// ExtendBaseimageCmd represents the baseimage command
type ExtendBaseimageCmd struct {
	cmd  *cobra.Command
	Opts *ExtendBaseimageOpts
}

type ExtendBaseimageOpts struct {
	*GlobalOptions
	Package string
	Force   bool
}

const baseimagePath = "./codesphere/images"
const defaultBaseimage = "workspace-agent-24.04.tar"

func (c *ExtendBaseimageCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.Package == "" {
		return errors.New("required option package not set")
	}

	workdir := env.NewEnv().GetOmsWorkdir()
	p := installer.NewPackage(workdir, c.Opts.Package)

	baseImageTarPath := path.Join(baseimagePath, defaultBaseimage)
	err := p.ExtractDependency(baseImageTarPath, c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	extractedBaseImagePath := p.GetDependencyPath(baseImageTarPath)
	d := system.NewDockerEngine()
	err = d.LoadLocalContainerImage(extractedBaseImagePath)
	if err != nil {
		return fmt.Errorf("failed to load baseimage file %s: %w", baseImageTarPath, err)
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
	}
	baseimage.cmd.Flags().StringVarP(&baseimage.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load base image from")
	baseimage.cmd.Flags().BoolVarP(&baseimage.Opts.Force, "force", "f", false, "Enforce package extraction")
	extend.AddCommand(baseimage.cmd)
	baseimage.cmd.RunE = baseimage.RunE
}
