// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"time"

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

// deployFlavors are the flavors deployment-service reads; the installer drops any others.
var deployFlavors = []string{"default", "gpu"}

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
) (map[string]any, error) {
	configured := conf.DeployConfig.Images
	if configured == nil {
		configured = defaultDeployImages(conf, imgs)
	}

	resolved := map[string]any{}

	for imageKey, image := range configured {
		value, err := deployImageValues(imageKey, image, registry, imgs)
		if err != nil {
			return nil, fmt.Errorf("deployConfig image %s: %w", imageKey, err)
		}

		resolved[imageKey] = value
	}

	return map[string]any{"images": resolved}, nil
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
	if _, err := imgs.Reference(defaultGPUAgentBOMRef); hasGPUAgent || err == nil {
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
			instances[id] = intOr(plan.PooledInstances, 0)
		}
	}

	return instances
}

// poolValues keys the pooled instances by plan id as a string, as values are JSON.
func poolValues(instances map[int]int) map[string]any {
	values := map[string]any{}
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
) (map[string]any, error) {
	supportedUntil, err := jsDateString(image.SupportedUntil)
	if err != nil {
		return nil, err
	}

	if _, ok := image.Flavors["default"]; !ok {
		return nil, fmt.Errorf("the default flavor is required")
	}

	flavors := map[string]any{}

	for _, flavorKey := range deployFlavors {
		flavor, ok := image.Flavors[flavorKey]
		if !ok {
			continue
		}

		resolved, err := flavorImage(imageKey, flavorKey, flavor.Image, registry, imgs)
		if err != nil {
			return nil, fmt.Errorf("flavor %s: %w", flavorKey, err)
		}

		flavors[flavorKey] = map[string]any{"image": resolved, "pool": poolValues(flavor.Pool)}
	}

	values := map[string]any{
		"name":           image.Name,
		"supportedUntil": supportedUntil,
		"flavors":        flavors,
	}
	if image.Default != nil {
		values["default"] = *image.Default
	}

	return values, nil
}

// flavorImage resolves a flavor's image. A flavor with a Dockerfile uses the image built from it,
// which the installer pushes to the registry as <image>-<flavor>.
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
		return "", fmt.Errorf("invalid supportedUntil %q: %w", date, err)
	}

	return parsed.UTC().Format("2006-01-02T15:04:05.000Z"), nil
}
