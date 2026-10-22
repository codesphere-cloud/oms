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
		named, err := imgs.Named(imageName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the %s image: %w", service, err)
		}

		services[service] = map[string]any{"image": named.Name()}
	}

	return services, nil
}

func workspaceRouterValues(imgs images.Images) (map[string]any, error) {
	nginx, err := imgs.Tagged("nginx")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the workspace router image: %w", err)
	}

	return map[string]any{
		"image":      map[string]any{"name": nginx.Name(), "tag": nginx.Tag()},
		"haReplicas": 2,
	}, nil
}

func workspaceObservabilityValues(imgs images.Images) (map[string]any, error) {
	opensearch, err := imgs.Tagged("opensearch")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the OpenSearch image: %w", err)
	}

	values := map[string]any{
		"openSearchImage":   opensearch.Name(),
		"openSearchVersion": opensearch.Tag(),
	}

	plain := map[string]string{
		"migrationKubectlImage":  "kubectl",
		"openSearchInitImage":    "opensearchInit",
		"otelCollectorImage":     "observabilityOtel",
		"otelCollectorInitImage": "observabilityOtelInit",
	}
	for key, imageName := range plain {
		image, err := imgs.Image(imageName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve the %s image: %w", key, err)
		}

		values[key] = image
	}

	return values, nil
}
