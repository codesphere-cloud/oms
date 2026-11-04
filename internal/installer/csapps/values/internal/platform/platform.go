// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package platform builds the values of the services and platform components: their images
// from the BOM and their fixed defaults.
package platform

import (
	"errors"
	"fmt"
	"maps"
	"slices"

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
	Image      images.Image `yaml:"image"`
	HAReplicas int          `yaml:"haReplicas"`
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

// Values resolves the images of the services and platform components from the BOM. It
// reports every image that fails, not just the first.
func Values(imgs images.Images) (Platform, error) {
	services, servicesErr := servicesValues(imgs)
	workspaceRouter, workspaceRouterErr := workspaceRouterValues(imgs)
	workspaceObservability, workspaceObservabilityErr := workspaceObservabilityValues(imgs)

	if err := errors.Join(servicesErr, workspaceRouterErr, workspaceObservabilityErr); err != nil {
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

	var errs []error

	for _, service := range slices.Sorted(maps.Keys(serviceImages)) {
		image, err := imgs.Name(serviceImages[service])
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to resolve the %s image: %w", service, err))
			continue
		}

		serviceImageNames[service] = ServiceImage{Image: image}
	}

	if err := errors.Join(errs...); err != nil {
		return Services{}, err
	}

	return Services{PriorityClass: PriorityClass, Images: serviceImageNames}, nil
}

func workspaceRouterValues(imgs images.Images) (WorkspaceRouter, error) {
	image, err := imgs.Image("nginx")
	if err != nil {
		return WorkspaceRouter{}, fmt.Errorf("failed to resolve the workspace router image: %w", err)
	}

	return WorkspaceRouter{Image: image, HAReplicas: workspaceRouterHAReplicas}, nil
}

func workspaceObservabilityValues(imgs images.Images) (WorkspaceObservability, error) {
	var errs []error

	openSearchName, openSearchTag, err := imgs.NameAndTag("opensearch")
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to resolve the OpenSearch image: %w", err))
	}

	openSearchInit, err := imgs.Reference("opensearchInit")
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to resolve the OpenSearch init image: %w", err))
	}

	kubectl, err := imgs.Reference("kubectl")
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to resolve the migration kubectl image: %w", err))
	}

	otelCollector, err := imgs.Reference("observabilityOtel")
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to resolve the OTel collector image: %w", err))
	}

	otelCollectorInit, err := imgs.Reference("observabilityOtelInit")
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to resolve the OTel collector init image: %w", err))
	}

	if err := errors.Join(errs...); err != nil {
		return WorkspaceObservability{}, err
	}

	return WorkspaceObservability{
		OpenSearchImage:        openSearchName,
		OpenSearchVersion:      openSearchTag,
		OpenSearchInitImage:    openSearchInit,
		MigrationKubectlImage:  kubectl,
		OtelCollectorImage:     otelCollector,
		OtelCollectorInitImage: otelCollectorInit,
	}, nil
}
