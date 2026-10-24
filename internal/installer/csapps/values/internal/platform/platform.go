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

// Platform are the images of the services and platform components.
type Platform struct {
	Services               Services               `yaml:"services"`
	WorkspaceRouter        WorkspaceRouter        `yaml:"workspaceRouter"`
	WorkspaceObservability WorkspaceObservability `yaml:"workspaceObservability"`
}

// Services are the Codesphere services, keyed as the chart names them.
type Services struct {
	PriorityClass string                  `yaml:"priorityClass"`
	Images        map[string]ServiceImage `yaml:",inline"`
}

// ServiceImage is a service's image name; the chart adds the release's tag.
type ServiceImage struct {
	Image string `yaml:"image"`
}

// WorkspaceRouter is the workspace router's image and replica count.
type WorkspaceRouter struct {
	Image      Image `yaml:"image"`
	HAReplicas int   `yaml:"haReplicas"`
}

// Image is an image reference split into name and tag.
type Image struct {
	Name string `yaml:"name"`
	Tag  string `yaml:"tag"`
}

// WorkspaceObservability are the images of the workspace observability stack.
type WorkspaceObservability struct {
	OpenSearchImage        string `yaml:"openSearchImage"`
	OpenSearchVersion      string `yaml:"openSearchVersion"`
	OpenSearchInitImage    string `yaml:"openSearchInitImage"`
	MigrationKubectlImage  string `yaml:"migrationKubectlImage"`
	OtelCollectorImage     string `yaml:"otelCollectorImage"`
	OtelCollectorInitImage string `yaml:"otelCollectorInitImage"`
}

// Values resolves the images of the services and platform components from the BOM.
func Values(imgs images.Images) (Platform, error) {
	services, err := servicesValues(imgs)
	if err != nil {
		return Platform{}, err
	}

	workspaceRouter, err := workspaceRouterValues(imgs)
	if err != nil {
		return Platform{}, err
	}

	workspaceObservability, err := workspaceObservabilityValues(imgs)
	if err != nil {
		return Platform{}, err
	}

	return Platform{
		Services:               services,
		WorkspaceRouter:        workspaceRouter,
		WorkspaceObservability: workspaceObservability,
	}, nil
}

func servicesValues(imgs images.Images) (Services, error) {
	serviceImageNames := make(map[string]ServiceImage, len(serviceImages))

	for service, imageName := range serviceImages {
		image, err := imgs.Name(imageName)
		if err != nil {
			return Services{}, fmt.Errorf("failed to resolve the %s image: %w", service, err)
		}

		serviceImageNames[service] = ServiceImage{Image: image}
	}

	return Services{PriorityClass: PriorityClass, Images: serviceImageNames}, nil
}

func workspaceRouterValues(imgs images.Images) (WorkspaceRouter, error) {
	image, err := nameAndTag(imgs, "nginx", "workspace router")
	if err != nil {
		return WorkspaceRouter{}, err
	}

	return WorkspaceRouter{Image: image, HAReplicas: workspaceRouterHAReplicas}, nil
}

func workspaceObservabilityValues(imgs images.Images) (WorkspaceObservability, error) {
	openSearch, err := nameAndTag(imgs, "opensearch", "OpenSearch")
	if err != nil {
		return WorkspaceObservability{}, err
	}

	openSearchInit, err := imgs.Reference("opensearchInit")
	if err != nil {
		return WorkspaceObservability{}, fmt.Errorf("failed to resolve the OpenSearch init image: %w", err)
	}

	kubectl, err := imgs.Reference("kubectl")
	if err != nil {
		return WorkspaceObservability{}, fmt.Errorf("failed to resolve the migration kubectl image: %w", err)
	}

	otelCollector, err := imgs.Reference("observabilityOtel")
	if err != nil {
		return WorkspaceObservability{}, fmt.Errorf("failed to resolve the OTel collector image: %w", err)
	}

	otelCollectorInit, err := imgs.Reference("observabilityOtelInit")
	if err != nil {
		return WorkspaceObservability{}, fmt.Errorf("failed to resolve the OTel collector init image: %w", err)
	}

	return WorkspaceObservability{
		OpenSearchImage:        openSearch.Name,
		OpenSearchVersion:      openSearch.Tag,
		OpenSearchInitImage:    openSearchInit,
		MigrationKubectlImage:  kubectl,
		OtelCollectorImage:     otelCollector,
		OtelCollectorInitImage: otelCollectorInit,
	}, nil
}

// nameAndTag resolves an image the chart takes as separate name and tag; purpose names it in errors.
func nameAndTag(imgs images.Images, name, purpose string) (Image, error) {
	image, err := imgs.Name(name)
	if err != nil {
		return Image{}, fmt.Errorf("failed to resolve the %s image: %w", purpose, err)
	}

	tag, err := imgs.Tag(name)
	if err != nil {
		return Image{}, fmt.Errorf("failed to resolve the %s image: %w", purpose, err)
	}

	return Image{Name: image, Tag: tag}, nil
}
