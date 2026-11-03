// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
)

// WaitForGatewayCRDs waits for the API types consumed by cluster setup. The
// gateway controller itself needs PriorityClasses installed by cluster setup,
// so requiring its Deployment or Argo Application to be healthy here deadlocks.
func (i *AppInstaller) WaitForGatewayCRDs(ctx context.Context, logf LogFunc) error {
	gateway := i.cfg.Config.Cluster.Gateway.GatewayAPI

	publicGateway := i.cfg.Config.Cluster.PublicGateway.GatewayAPI
	if gateway != nil && !gateway.Enabled && publicGateway != nil && !publicGateway.Enabled {
		return nil
	}

	requirements := []installer.CRDRequirement{
		{Name: "gatewayclasses.gateway.networking.k8s.io", Version: "v1"},
		{Name: "gateways.gateway.networking.k8s.io", Version: "v1"},
		{Name: "httproutes.gateway.networking.k8s.io", Version: "v1"},
		{Name: "envoyproxies.gateway.envoyproxy.io", Version: "v1alpha1"},
	}
	if err := installer.WaitForCRDs(ctx, i.cfg.KubeClient, requirements, applicationReadyTimeout, logf); err != nil {
		return fmt.Errorf("waiting for Gateway APIs: %w", err)
	}

	return nil
}
