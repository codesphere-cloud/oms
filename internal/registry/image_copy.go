// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
)

type ImageCopier interface {
	Copy(sourceRef string, destinationRef string) error
}

type RegistryImageCopier struct {
	ctx context.Context
}

func NewRegistryImageCopier(ctx context.Context) *RegistryImageCopier {
	return &RegistryImageCopier{ctx: ctx}
}

func (c *RegistryImageCopier) Copy(sourceRef string, destinationRef string) error {
	if err := crane.Copy(sourceRef, destinationRef, crane.WithContext(c.ctx)); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", sourceRef, destinationRef, err)
	}

	return nil
}
