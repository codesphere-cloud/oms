// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

import (
	"fmt"
	"time"

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

func deployConfigValues(cfg *files.RootConfig, images bomImages) (map[string]any, error) {
	configured := cfg.Codesphere.DeployConfig.Images
	if len(configured) == 0 {
		configured = defaultDeployImages(cfg.Codesphere, images)
	}

	resolved := map[string]any{}

	for imageKey, image := range configured {
		value, err := deployImageValues(imageKey, image, cfg.Registry, images)
		if err != nil {
			return nil, fmt.Errorf("deployConfig image %s: %w", imageKey, err)
		}

		resolved[imageKey] = value
	}

	return map[string]any{"images": resolved}, nil
}

func defaultDeployImages(conf files.CodesphereConfig, images bomImages) map[string]files.ImageConfig {
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
		"default": {Image: agent, Pool: pool(conf.Plans.HostingPlans, func(p files.HostingPlan) bool { return p.GPUParts <= 0 })},
	}
	if _, err := images.image(defaultGPUAgentBOMRef); hasGPUAgent || err == nil {
		flavors["gpu"] = files.FlavorConfig{Image: gpuAgent, Pool: pool(conf.Plans.HostingPlans, func(p files.HostingPlan) bool { return p.GPUParts > 0 })}
	}

	return map[string]files.ImageConfig{
		defaultImageKey: {Name: defaultImageName, SupportedUntil: defaultImageSupportedUntil, Flavors: flavors},
	}
}

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

func deployImageValues(imageKey string, image files.ImageConfig, registry *files.RegistryConfig, images bomImages) (map[string]any, error) {
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

		resolved, err := flavorImage(imageKey, flavorKey, flavor.Image, registry, images)
		if err != nil {
			return nil, fmt.Errorf("flavor %s: %w", flavorKey, err)
		}

		flavors[flavorKey] = map[string]any{"image": resolved, "pool": poolValues(flavor.Pool)}
	}

	values := map[string]any{"name": image.Name, "supportedUntil": supportedUntil, "flavors": flavors}
	if image.Default != nil {
		values["default"] = *image.Default
	}

	return values, nil
}

// flavorImage resolves a flavor's image. A flavor with a Dockerfile uses the image built from it,
// which the installer pushes to the registry as <image>-<flavor>.
func flavorImage(imageKey, flavorKey string, image files.ImageRef, registry *files.RegistryConfig, images bomImages) (string, error) {
	if image.ImageName == "" && image.Dockerfile != "" && registry != nil && registry.Server != "" {
		return fmt.Sprintf("%s/%s-%s", registry.Server, imageKey, flavorKey), nil
	}

	return images.workspaceImage(image)
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

func workspaceImagesValues(configured *files.WorkspaceImagesConfig, images bomImages) (map[string]any, error) {
	if configured == nil {
		configured = &files.WorkspaceImagesConfig{}
	}

	pick := func(image *files.ImageRef, bomRef string) files.ImageRef {
		if image != nil {
			return *image
		}

		return files.ImageRef{BomRef: bomRef}
	}

	values := map[string]any{}
	// The agents are optional: a BOM without them leaves them out.
	if agent, err := images.workspaceImage(pick(configured.Agent, "workspace-agent")); err == nil {
		values["agent"] = agent
	}

	if agentGPU, err := images.workspaceImage(pick(configured.AgentGpu, "workspace-agent-gpu")); err == nil {
		values["agentGpu"] = agentGPU
	}

	server, err := images.workspaceImage(pick(configured.Server, "workspace-server"))
	if err != nil {
		return nil, err
	}

	vpn, err := images.workspaceImage(pick(configured.VPN, "vpn"))
	if err != nil {
		return nil, err
	}

	values["server"] = server
	values["vpn"] = vpn

	return values, nil
}

func servicesValues(images bomImages) (map[string]any, error) {
	services := map[string]any{"priorityClass": prodPriorityClass}

	for service, imageName := range serviceImages {
		ref, err := images.ref(imageName)
		if err != nil {
			return nil, err
		}

		services[service] = map[string]any{"image": ref.image}
	}

	return services, nil
}

func workspaceRouterValues(images bomImages) (map[string]any, error) {
	nginx, err := images.ref("nginx")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"image":      map[string]any{"name": nginx.image, "tag": nginx.tag},
		"haReplicas": 2,
	}, nil
}

func frontendGatewayValues(images bomImages, issuer *files.CertIssuerConfig, gatewayAPIEnabled bool) (map[string]any, error) {
	nginx, err := images.ref("frontendGatewayNginx")
	if err != nil {
		return nil, err
	}

	issuerName := selfSignedIssuerName
	if issuer != nil && issuer.Type != "" {
		issuerName = string(issuer.Type)
	}

	return map[string]any{
		"issuer":        issuerName,
		"redirectToIde": true,
		"image":         map[string]any{"name": nginx.image, "tag": nginx.tag},
		"gatewayApi":    map[string]any{"enabled": gatewayAPIEnabled},
	}, nil
}

func workspaceObservabilityValues(images bomImages) (map[string]any, error) {
	opensearch, err := images.ref("opensearch")
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"openSearchImage":   opensearch.image,
		"openSearchVersion": opensearch.tag,
	}

	plain := map[string]string{
		"migrationKubectlImage":  "kubectl",
		"openSearchInitImage":    "opensearchInit",
		"otelCollectorImage":     "observabilityOtel",
		"otelCollectorInitImage": "observabilityOtelInit",
	}
	for key, imageName := range plain {
		image, err := images.image(imageName)
		if err != nil {
			return nil, err
		}

		values[key] = image
	}

	return values, nil
}
