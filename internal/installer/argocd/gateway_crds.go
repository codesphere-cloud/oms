// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"
)

// WaitForGatewayCRDs waits for API definitions, rather than controllers whose
// startup depends on PriorityClasses created later by cluster setup.
func (i *AppInstaller) WaitForGatewayCRDs(ctx context.Context, logf LogFunc) error {
	gateway := i.cfg.Config.Cluster.Gateway.GatewayAPI

	publicGateway := i.cfg.Config.Cluster.PublicGateway.GatewayAPI
	if gateway != nil && !gateway.Enabled && publicGateway != nil && !publicGateway.Enabled {
		return nil
	}

	if i.cfg.HelmClient == nil {
		return fmt.Errorf("helm client is required to wait for Gateway CRDs")
	}

	names := []string{
		"gatewayclasses.gateway.networking.k8s.io",
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"envoyproxies.gateway.envoyproxy.io",
	}
	if logf != nil {
		logf("Waiting for Gateway CRDs to be ready: %v", names)
	}

	if err := i.cfg.HelmClient.WaitForResources(ctx, "apiextensions.k8s.io/v1", "CustomResourceDefinition", names, applicationReadyTimeout); err != nil {
		return fmt.Errorf("waiting for Gateway CRDs: %w", err)
	}

	return nil
}
