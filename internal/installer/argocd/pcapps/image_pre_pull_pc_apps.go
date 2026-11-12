// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	// Registers SHA-256 so reference.Parse accepts sha256 digests.
	_ "crypto/sha256"
	"errors"
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/distribution/reference"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// imagePrePullMinChartVersion is the first pc-applications chart version with the image-pre-pull
// application. OMS also installs older packages, and an application the chart does not know breaks the
// sync. The chart's values are an OCI artifact OMS does not load, so the BOM's chart version is checked.
const imagePrePullMinChartVersion = "0.442.0"

const pcAppsChart = "pc-applications"

// maxContainerNameLength is the longest container name: the chart adds "pull-" to a name and
// a DNS-1123 label has at most 63 characters.
const maxContainerNameLength = 58

var nonLabelChars = regexp.MustCompile(`[^a-z0-9-]+`)

// ImagePrePullPcAppsValues enables the image-pre-pull application and gives it the images the
// Codesphere install uses. They are read from the Codesphere values (csappsvalues.Build), so
// both installs agree. Returns nil when the app is not enabled: the install config turns
// pre-pulling off, the pcApps block disables the app, the BOM's pc-applications chart is too old,
// or the values cannot be built. Callers that turn off the umbrella pre-pull must use this result
// as the one decision whether the app is on.
//
// The pre-pull is optional, so a problem logs a warning and skips it, it never fails the install.
//
// The pre-pull lives in pc-applications, which OMS does not wait on. A slow rollout of the
// DaemonSets does not block the install.
//
// The app is central. With several Codesphere releases on one cluster (dev previews), only the
// images of the release OMS installs are pre-pulled.
func ImagePrePullPcAppsValues(config *files.RootConfig, bomConfig *bom.Config) files.ChartValues {
	if enabled, set := lookup(config.PcApps, "applications", "image-pre-pull", "enabled").(bool); set && !enabled {
		return nil
	}

	built, err := csappsvalues.Build(config, bomConfig)
	if err != nil {
		log.Printf("Warning: skipping image pre-pull, cannot build the Codesphere values: %v\n", err)
		return nil
	}

	global, _ := built["global"].(map[string]any)
	if enabled, _ := global["prePullImages"].(bool); !enabled {
		return nil
	}

	if !chartHasImagePrePull(bomConfig) {
		log.Printf("Warning: skipping image pre-pull, the %s chart in the BOM is older than %s\n", pcAppsChart, imagePrePullMinChartVersion)
		return nil
	}

	values, err := imagePrePullValues(global, bomConfig)
	if err != nil {
		log.Printf("Warning: skipping image pre-pull: %v\n", err)
		return nil
	}

	return values
}

// chartHasImagePrePull reports whether the BOM's pc-applications chart has the image-pre-pull application.
func chartHasImagePrePull(bomConfig *bom.Config) bool {
	chart, ok := bomConfig.GetChart(pcAppsChart)
	if !ok {
		return false
	}

	version, err := semver.NewVersion(chart.Tag())
	if err != nil {
		return false
	}

	return !version.LessThan(semver.MustParse(imagePrePullMinChartVersion))
}

func imagePrePullValues(global map[string]any, bomConfig *bom.Config) (files.ChartValues, error) {
	imageTag, _ := global["imageTag"].(string)

	bomImages, err := bomConfig.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	workspaceImages := workspacePrePullImages(global, bomImages, imageTag)

	// The chart needs the pause image. It is the workspace server image.
	pauseImage := pausePrePullImage(workspaceImages)
	if pauseImage == "" {
		return nil, errors.New("there is no workspace server image")
	}

	priorityClass, _ := lookup(global, "services", "priorityClass").(string)
	allowOnControlPlane, _ := global["allowWorkspacesOnControlPlane"].(bool)

	return files.ChartValues{
		"applications": files.ChartValues{
			"image-pre-pull": files.ChartValues{
				"enabled": true,
				"valuesObject": files.ChartValues{
					"priorityClassName":   priorityClass,
					"allowOnControlPlane": allowOnControlPlane,
					"pauseImage":          pauseImage,
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

		tagged := imageWithTag(name, imageTag)
		if tagged == "" {
			continue
		}

		images = append(images, prePullImage{"name": workspaceImage.name, "image": tagged})
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
// Only the default flavor is pulled, as the gpu flavor only runs on gpu nodes.
func agentPrePullImages(global map[string]any, imageTag string) []prePullImage {
	osImages, _ := lookup(global, "deployConfig", "images").(map[string]any)
	images := []prePullImage{}
	usedNames := map[string]bool{}

	for index, osName := range slices.Sorted(maps.Keys(osImages)) {
		image, _ := lookup(osImages[osName], "flavors", "default", "image").(string)

		tagged := imageWithTag(image, imageTag)
		if tagged == "" {
			continue
		}

		images = append(images, prePullImage{"name": agentContainerName(osName, index, usedNames), "image": tagged})
	}

	return images
}

// agentContainerName returns a DNS-1123 label for the agent of an OS key that is not in usedNames and
// fits the chart's length limit. A name that is taken gets the index of its OS as suffix.
func agentContainerName(osName string, index int, usedNames map[string]bool) string {
	base := strings.Trim(nonLabelChars.ReplaceAllString(strings.ToLower(osName), "-"), "-")
	name := truncateName("agent-"+base+"-default", maxContainerNameLength)

	if usedNames[name] {
		suffix := "-" + strconv.Itoa(index)
		name = truncateName("agent-"+base+"-default", maxContainerNameLength-len(suffix)) + suffix
	}

	usedNames[name] = true

	return name
}

// truncateName cuts name to at most length characters, without a dash at the end.
func truncateName(name string, length int) string {
	if len(name) > length {
		name = name[:length]
	}

	return strings.TrimRight(name, "-")
}

// imageWithTag adds the tag to an image, unless the image has a tag or digest of its own.
// Returns "" if the image is empty or invalid.
func imageWithTag(image, tag string) string {
	parsed, err := reference.Parse(image)
	if err != nil {
		return ""
	}

	switch parsed.(type) {
	case reference.Tagged, reference.Digested:
		return image
	}

	return image + ":" + tag
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
