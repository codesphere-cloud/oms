// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/portal"
)

// PullImageCmd represents the pull image command
type PullImageCmd struct {
	cmd  *cobra.Command
	Opts PullImageOpts
}

type PullImageOpts struct {
	*GlobalOptions
	OutputDir string
}

func (c *PullImageCmd) RunE(_ *cobra.Command, args []string) error {
	imageRef, err := portal.ParseImageRef(args[0])
	if err != nil {
		return fmt.Errorf("invalid image reference: %w", err)
	}

	log.Printf("Pulling image %s/%s:%s", imageRef.Org, imageRef.Repo, imageRef.Tag)
	log.Printf("Output directory: %s", c.Opts.OutputDir)

	oci := portal.NewOCIClient()
	if err := oci.PullImage(imageRef, c.Opts.OutputDir); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	log.Printf("Image pulled successfully to %s", c.Opts.OutputDir)
	return nil
}

func AddPullImageCmd(pull *cobra.Command, opts *GlobalOptions) {
	image := PullImageCmd{
		cmd: &cobra.Command{
			Use:   "image IMAGE_REF",
			Short: "Pull a container image through the OMS portal registry proxy",
			Long: io.Long(`Pull an OCI container image through the OMS portal's GHCR registry proxy.
				Downloads all layers and the image config to a local directory.

				The IMAGE_REF must be in the format org/repo:tag (e.g. codesphere-cloud/some-image:v1.0.0).

				Requires OMS_PORTAL_API_KEY to be set in the environment.`),
			Args: cobra.ExactArgs(1),
			Example: formatExamples("pull image", []io.Example{
				{Cmd: "codesphere-cloud/my-image:v1.0.0", Desc: "Pull a specific image tag"},
				{Cmd: "codesphere-cloud/my-image:v1.0.0 --output ./layers", Desc: "Pull to a custom output directory"},
			}),
		},
		Opts: PullImageOpts{GlobalOptions: opts, OutputDir: "./oci-layers"},
	}
	image.cmd.Flags().StringVarP(&image.Opts.OutputDir, "output", "o", "./oci-layers", "Output directory for downloaded OCI layers")

	AddCmd(pull, image.cmd)

	image.cmd.RunE = image.RunE
}
