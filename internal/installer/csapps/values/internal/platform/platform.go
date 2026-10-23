// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package platform builds the values of the platform images, which come from the BOM only.
package platform

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
)

// PriorityClass is the priority class of the Codesphere services.
const PriorityClass = "prod-codesphere-deployment"

// workspaceRouterHAReplicas is the replica count the Codesphere installer gives the workspace router.
const workspaceRouterHAReplicas = 2

// serviceImages maps each service in the chart to its image in the BOM.
var serviceImages = map[string]string{
	"auth_service":       "auth-service",
	"deployment_service": "deployment-service",
	"error_page_server":  "error-page-server",
	"ide_frontend":       "ide-frontend",
	"ide_service":        "ide-service",
	"marketplace":        "marketplace",
	"payment_service":    "payment-service",
	"public_api_service": "public-api-service",
	"team_service":       "team-service",
	"workspace_proxy":    "workspace-proxy",
	"workspace_service":  "workspace-service",
}

// Values are the images of the services and platform components, which come from the
// BOM only.
func Values(imgs images.Images) (map[string]any, error) {
	services, err := servicesValues(imgs)
	if err != nil {
		return nil, err
	}

	workspaceRouter, err := workspaceRouterValues(imgs)
	if err != nil {
		return nil, err
	}

	workspaceObservability, err := workspaceObservabilityValues(imgs)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"services":               services,
		"workspaceRouter":        workspaceRouter,
		"workspaceObservability": workspaceObservability,
	}, nil
}

func servicesValues(imgs images.Images) (map[string]any, error) {
	services := map[string]any{"priorityClass": PriorityClass}

	for service, imageName := range serviceImages {
		image, err := imgs.Name(imageName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the %s image: %w", service, err)
		}

		services[service] = map[string]any{"image": image}
	}

	return services, nil
}

func workspaceRouterValues(imgs images.Images) (map[string]any, error) {
	image, err := imgs.Name("nginx")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the workspace router image: %w", err)
	}

	tag, err := imgs.Tag("nginx")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the workspace router image: %w", err)
	}

	return map[string]any{
		"image":      map[string]any{"name": image, "tag": tag},
		"haReplicas": workspaceRouterHAReplicas,
	}, nil
}

func workspaceObservabilityValues(imgs images.Images) (map[string]any, error) {
	openSearchImage, err := imgs.Name("opensearch")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the OpenSearch image: %w", err)
	}

	openSearchVersion, err := imgs.Tag("opensearch")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the OpenSearch image: %w", err)
	}

	values := map[string]any{
		"openSearchImage":   openSearchImage,
		"openSearchVersion": openSearchVersion,
	}

	plain := map[string]string{
		"migrationKubectlImage":  "kubectl",
		"openSearchInitImage":    "opensearchInit",
		"otelCollectorImage":     "observabilityOtel",
		"otelCollectorInitImage": "observabilityOtelInit",
	}
	for key, imageName := range plain {
		image, err := imgs.Reference(imageName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the %s image: %w", key, err)
		}

		values[key] = image
	}

	return values, nil
}
