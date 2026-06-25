// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system

import (
	"context"
	"fmt"
	"os"

	imagecopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/signature"
)

type RegistryImageCopier struct {
	ctx context.Context
}

func NewRegistryImageCopier(ctx context.Context) *RegistryImageCopier {
	return &RegistryImageCopier{ctx: ctx}
}

func (c *RegistryImageCopier) CopyAll(sourceRef string, destinationRef string) error {
	source, err := docker.ParseReference("//" + sourceRef)
	if err != nil {
		return fmt.Errorf("failed to parse source image reference %s: %w", sourceRef, err)
	}

	destination, err := docker.ParseReference("//" + destinationRef)
	if err != nil {
		return fmt.Errorf("failed to parse destination image reference %s: %w", destinationRef, err)
	}

	policy, err := signature.NewPolicyContext(&signature.Policy{
		Default: []signature.PolicyRequirement{
			signature.NewPRInsecureAcceptAnything(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create image signature policy: %w", err)
	}
	defer policy.Destroy()

	_, err = imagecopy.Image(c.ctx, policy, destination, source, &imagecopy.Options{
		ImageListSelection: imagecopy.CopyAllImages,
		ReportWriter:       os.Stdout,
		RemoveSignatures:   true,
	})
	if err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", sourceRef, destinationRef, err)
	}

	return nil
}
