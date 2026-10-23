// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// workspaceImage resolves a configured workspace image to an image name without tag.
func workspaceImage(imgs images.Images, configured files.ImageRef) (string, error) {
	if configured.ImageName != "" {
		return configured.ImageName, nil
	}

	image, err := imgs.Name(configured.BomRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve BOM image %s: %w", configured.BomRef, err)
	}

	return image, nil
}

// WorkspaceImages are the images every workspace runs besides its agent.
type WorkspaceImages struct {
	Server string `yaml:"server"`
	VPN    string `yaml:"vpn"`
}

func workspaceImagesValues(
	configured *files.WorkspaceImagesConfig,
	imgs images.Images,
) (WorkspaceImages, error) {
	if configured == nil {
		configured = &files.WorkspaceImagesConfig{}
	}

	pick := func(image *files.ImageRef, bomRef string) files.ImageRef {
		if image != nil {
			return *image
		}

		return files.ImageRef{BomRef: bomRef}
	}

	server, err := workspaceImage(imgs, pick(configured.Server, "workspace-server"))
	if err != nil {
		return WorkspaceImages{}, fmt.Errorf("failed to resolve the workspace server image: %w", err)
	}

	vpn, err := workspaceImage(imgs, pick(configured.VPN, "vpn"))
	if err != nil {
		return WorkspaceImages{}, fmt.Errorf("failed to resolve the workspace VPN image: %w", err)
	}

	return WorkspaceImages{Server: server, VPN: vpn}, nil
}
