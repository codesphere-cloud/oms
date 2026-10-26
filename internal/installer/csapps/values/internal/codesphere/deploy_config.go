// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"time"

	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	defaultImageKey            = "ubuntu-24.04"
	defaultImageName           = "Ubuntu 24.04"
	defaultImageSupportedUntil = "2028-05-31"
	defaultAgentBOMRef         = "workspace-agent-24.04"
	defaultGPUAgentBOMRef      = "workspace-agent-24.04.gpu"
)

var deployFlavors = []string{"default", "gpu"}

// DeployConfig are the images workspaces deploy with.
type DeployConfig struct {
	Images map[string]DeployImage `yaml:"images"`
}

// DeployImage is a workspace image and its flavors.
type DeployImage struct {
	Name           string            `yaml:"name"`
	SupportedUntil string            `yaml:"supportedUntil"`
	Flavors        map[string]Flavor `yaml:"flavors"`
	Default        *bool             `yaml:"default,omitempty"`
}

// Flavor is a flavor's image and the instances pooled for it per plan id.
type Flavor struct {
	Image string         `yaml:"image"`
	Pool  map[string]int `yaml:"pool"`
}

func validateDeployConfig(conf files.CodesphereConfig) error {
	if conf.DeployConfig.Images == nil {
		return nil
	}

	for _, plan := range conf.Plans.HostingPlans {
		if plan.PooledInstances != nil && *plan.PooledInstances != 0 {
			return fmt.Errorf(
				"configuring .deployConfig AND .pooledInstances is ambiguous (remove the .pooledInstances config)",
			)
		}
	}

	return nil
}

func deployConfigValues(
	conf files.CodesphereConfig,
	registry *files.RegistryConfig,
	imgs images.Images,
) (DeployConfig, error) {
	if err := validateDeployConfig(conf); err != nil {
		return DeployConfig{}, err
	}

	configured := conf.DeployConfig.Images
	if configured == nil {
		configured = defaultDeployImages(conf, imgs)
	}

	resolved := make(map[string]DeployImage, len(configured))

	for imageKey, image := range configured {
		value, err := deployImageValues(imageKey, image, registry, imgs)
		if err != nil {
			return DeployConfig{}, fmt.Errorf("deployConfig image %s: %w", imageKey, err)
		}

		resolved[imageKey] = value
	}

	return DeployConfig{Images: resolved}, nil
}

func defaultDeployImages(
	conf files.CodesphereConfig,
	imgs images.Images,
) map[string]files.ImageConfig {
	agent := files.ImageRef{BomRef: defaultAgentBOMRef}
	gpuAgent := files.ImageRef{BomRef: defaultGPUAgentBOMRef}

	if conf.WorkspaceImages != nil && conf.WorkspaceImages.Agent != nil {
		agent = *conf.WorkspaceImages.Agent
	}

	hasGPUAgent := conf.WorkspaceImages != nil && conf.WorkspaceImages.AgentGpu != nil
	if hasGPUAgent {
		gpuAgent = *conf.WorkspaceImages.AgentGpu
	}

	flavors := map[string]files.FlavorConfig{
		"default": {Image: agent, Pool: pool(conf.Plans.HostingPlans, withoutGPU)},
	}

	bomHasGPUAgent := imgs[defaultGPUAgentBOMRef] != ""
	if hasGPUAgent || bomHasGPUAgent {
		flavors["gpu"] = files.FlavorConfig{
			Image: gpuAgent,
			Pool:  pool(conf.Plans.HostingPlans, withGPU),
		}
	}

	return map[string]files.ImageConfig{
		defaultImageKey: {
			Name:           defaultImageName,
			SupportedUntil: defaultImageSupportedUntil,
			Flavors:        flavors,
		},
	}
}

func withGPU(plan files.HostingPlan) bool { return plan.GPUParts > 0 }

func withoutGPU(plan files.HostingPlan) bool { return plan.GPUParts <= 0 }

func pool(plans map[int]files.HostingPlan, include func(files.HostingPlan) bool) map[int]int {
	instances := map[int]int{}

	for id, plan := range plans {
		if include(plan) {
			instances[id] = ptr.Deref(plan.PooledInstances, 0)
		}
	}

	return instances
}

// poolValues keys the pooled instances by plan id as a string, as values are JSON.
func poolValues(instances map[int]int) map[string]int {
	values := make(map[string]int, len(instances))
	for planID, count := range instances {
		values[fmt.Sprint(planID)] = count
	}

	return values
}

func deployImageValues(
	imageKey string,
	image files.ImageConfig,
	registry *files.RegistryConfig,
	imgs images.Images,
) (DeployImage, error) {
	supportedUntil, err := jsDateString(image.SupportedUntil)
	if err != nil {
		return DeployImage{}, fmt.Errorf("supportedUntil: %w", err)
	}

	if _, ok := image.Flavors["default"]; !ok {
		return DeployImage{}, fmt.Errorf("the default flavor is required")
	}

	flavors := map[string]Flavor{}

	for _, flavorKey := range deployFlavors {
		flavor, ok := image.Flavors[flavorKey]
		if !ok {
			continue
		}

		resolved, err := flavorImage(imageKey, flavorKey, flavor.Image, registry, imgs)
		if err != nil {
			return DeployImage{}, fmt.Errorf("flavor %s: %w", flavorKey, err)
		}

		flavors[flavorKey] = Flavor{Image: resolved, Pool: poolValues(flavor.Pool)}
	}

	return DeployImage{
		Name:           image.Name,
		SupportedUntil: supportedUntil,
		Flavors:        flavors,
		Default:        image.Default,
	}, nil
}

// flavorImage resolves a flavor's image. A flavor with a Dockerfile uses the image built from it,
// which `oms build-images` pushes to the registry as <image>-<flavor>.
func flavorImage(
	imageKey, flavorKey string,
	image files.ImageRef,
	registry *files.RegistryConfig,
	imgs images.Images,
) (string, error) {
	if image.ImageName == "" && image.Dockerfile != "" && registry != nil && registry.Server != "" {
		return fmt.Sprintf("%s/%s-%s", registry.Server, imageKey, flavorKey), nil
	}

	resolved, err := workspaceImage(imgs, image)
	if err != nil {
		return "", fmt.Errorf("failed to resolve the workspace image: %w", err)
	}

	return resolved, nil
}

// jsDateString formats a date like the installer's values, which serialize a JavaScript Date.
func jsDateString(date string) (string, error) {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, date)
	}

	if err != nil {
		return "", fmt.Errorf("invalid date %q: %w", date, err)
	}

	return parsed.UTC().Format("2006-01-02T15:04:05.000Z"), nil
}
