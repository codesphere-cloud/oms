// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	if logf == nil {
		logf = func(string, ...interface{}) {}
	}

	var pending []string

	err := wait.PollUntilContextTimeout(ctx, applicationReadyPollInterval, applicationReadyTimeout, true, func(ctx context.Context) (bool, error) {
		var err error

		pending, err = pendingGatewayCRDs(ctx, i.cfg.KubeClient)
		if err != nil {
			return false, err
		}

		if len(pending) == 0 {
			logf("Gateway CRDs are Established and serve the required API versions")
			return true, nil
		}

		logf("Waiting for Gateway CRDs to become Established and serve required API versions: %v", pending)

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for Gateway CRDs (pending: %v): %w", pending, err)
	}

	return nil
}

func pendingGatewayCRDs(ctx context.Context, kubeClient client.Client) ([]string, error) {
	requirements := []struct{ name, version string }{
		{"gatewayclasses.gateway.networking.k8s.io", "v1"},
		{"gateways.gateway.networking.k8s.io", "v1"},
		{"httproutes.gateway.networking.k8s.io", "v1"},
		{"envoyproxies.gateway.envoyproxy.io", "v1alpha1"},
	}

	var pending []string

	for _, requirement := range requirements {
		crd := &apiextensionsv1.CustomResourceDefinition{}

		err := kubeClient.Get(ctx, client.ObjectKey{Name: requirement.name}, crd)
		if apierrors.IsNotFound(err) {
			pending = append(pending, requirement.name)
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("reading Gateway CRD %q: %w", requirement.name, err)
		}

		established := false

		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
				established = true
			}
		}

		served := false

		for _, version := range crd.Spec.Versions {
			if version.Name == requirement.version && version.Served {
				served = true
			}
		}

		if !established || !served {
			pending = append(pending, requirement.name)
		}
	}

	return pending, nil
}
