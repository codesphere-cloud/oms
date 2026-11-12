// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/distribution/reference"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// ImagePrePullPcAppsValues enables the image-pre-pull application and gives it the images the
// Codesphere install uses. They are read from the Codesphere values (csappsvalues.Build), so
// both installs agree. Returns nil when the install config turns pre-pulling off.
//
// The pre-pull lives in pc-applications, which OMS does not wait on. A slow rollout of the
// DaemonSets does not block the install.
//
// The app is central. With several Codesphere releases on one cluster (dev previews), only the
// images of the release OMS installs are pre-pulled.
func ImagePrePullPcAppsValues(config *files.RootConfig, bomConfig *bom.Config) (files.ChartValues, error) {
	built, err := csappsvalues.Build(config, bomConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build the Codesphere values: %w", err)
	}

	global, _ := built["global"].(map[string]any)
	if enabled, _ := global["prePullImages"].(bool); !enabled {
		return nil, nil
	}

	imageTag, _ := global["imageTag"].(string)

	bomImages, err := bomConfig.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	priorityClass, _ := lookup(global, "services", "priorityClass").(string)
	allowOnControlPlane, _ := global["allowWorkspacesOnControlPlane"].(bool)

	workspaceImages := workspacePrePullImages(global, bomImages, imageTag)

	return files.ChartValues{
		"applications": files.ChartValues{
			"image-pre-pull": files.ChartValues{
				"enabled": true,
				"valuesObject": files.ChartValues{
					"priorityClassName":   priorityClass,
					"allowOnControlPlane": allowOnControlPlane,
					"pauseImage":          pausePrePullImage(workspaceImages),
					"images": files.ChartValues{
						"workspace": workspaceImages,
						"agents":    agentPrePullImages(global, imageTag),
					},
				},
			},
		},
	}, nil
}

// prePullImage is one image of the chart's images lists.
type prePullImage = map[string]string

// workspacePrePullImages lists the workspace images in the order the umbrella chart pulls them. The
// Codesphere values only hold server and vpn. The router and the custom workspace image init come
// from an override or else from the BOM, where the chart default points to as well. An image
// that is in neither is left out.
func workspacePrePullImages(global map[string]any, bomImages map[string]string, imageTag string) []prePullImage {
	images := []prePullImage{}

	for _, workspaceImage := range []struct{ name, key, bomRef string }{
		{"workspace-server", "server", "workspace-server"},
		{"vpn", "vpn", "vpn"},
		{"custom-workspace-image-init", "customWorkspaceImageInit", "custom-workspace-image-init"},
		{"workspace-router", "router", "workspace-router"},
	} {
		name, _ := lookup(global, "workspaceImages", workspaceImage.key).(string)
		if name == "" {
			name = bomImageName(bomImages[workspaceImage.bomRef])
		}

		if name == "" {
			continue
		}

		images = append(images, prePullImage{"name": workspaceImage.name, "image": name + ":" + imageTag})
	}

	return images
}

// pausePrePullImage returns the image of the long-running pause container. It is the workspace
// server image, as it has sh and sleep. Returns "" if the list has no workspace server.
func pausePrePullImage(workspaceImages []prePullImage) string {
	for _, image := range workspaceImages {
		if image["name"] == "workspace-server" {
			return image["image"]
		}
	}

	return ""
}

// agentPrePullImages lists the default flavor image of every OS in the deploy config, sorted by OS.
func agentPrePullImages(global map[string]any, imageTag string) []prePullImage {
	osImages, _ := lookup(global, "deployConfig", "images").(map[string]any)
	images := []prePullImage{}

	for _, osName := range slices.Sorted(maps.Keys(osImages)) {
		image, _ := lookup(osImages[osName], "flavors", "default", "image").(string)
		if image == "" {
			continue
		}

		images = append(images, prePullImage{
			"name":  "agent-" + strings.ReplaceAll(osName, ".", "-") + "-default",
			"image": image + ":" + imageTag,
		})
	}

	return images
}

// bomImageName returns the image of a BOM reference without tag and digest, or "" if it has none.
func bomImageName(ref string) string {
	parsed, err := reference.Parse(ref)
	if err != nil {
		return ""
	}

	named, ok := parsed.(reference.Named)
	if !ok {
		return ""
	}

	return named.Name()
}

// lookup returns the value at the path of nested maps, or nil if the path breaks.
func lookup(value any, path ...string) any {
	for _, key := range path {
		m, ok := value.(map[string]any)
		if !ok {
			return nil
		}

		value = m[key]
	}

	return value
}
