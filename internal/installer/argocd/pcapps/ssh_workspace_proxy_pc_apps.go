// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// SSHWorkspaceProxyPcAppsValues derives the pc-applications values for the SSH workspace proxy.
// The proxy and its service are always enabled; the service settings come from the
// codesphere.sshWorkspaceProxy.service block and are left to the chart defaults when it is unset.
// They are the base of the pc-apps values: an explicit `pcApps` block in config.yaml and any
// --pc-apps-values file still override them.
func SSHWorkspaceProxyPcAppsValues(config *files.RootConfig) files.ChartValues {
	return files.ChartValues{
		"applications": files.ChartValues{
			"ssh-workspace-proxy": files.ChartValues{
				"enabled": true,
				"valuesObject": files.ChartValues{
					"service": sshWorkspaceProxyServiceValues(config.Codesphere.SSHWorkspaceProxy.ServiceConfig()),
				},
			},
		},
	}
}

// sshWorkspaceProxyServiceValues always enables the service. A load balancer keeps the client
// source IP (externalTrafficPolicy Local) and may request a fixed IP.
func sshWorkspaceProxyServiceValues(service *files.SSHWorkspaceProxyServiceConfig) files.ChartValues {
	values := files.ChartValues{"enabled": true}
	if service == nil {
		return values
	}

	values["type"] = service.ServiceType()

	if service.UsesNodePort() {
		values["nodePort"] = service.ServiceNodePort()
	}

	if service.IsLoadBalancer() {
		values["externalTrafficPolicy"] = "Local"
	}

	if service.IsLoadBalancer() && service.LoadBalancerIP != "" {
		values["loadBalancerIP"] = service.LoadBalancerIP
	}

	if len(service.Annotations) > 0 {
		annotations := files.ChartValues{}
		for key, value := range service.Annotations {
			annotations[key] = value
		}

		values["annotations"] = annotations
	}

	return values
}
