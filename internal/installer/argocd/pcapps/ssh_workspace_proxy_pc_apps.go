// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// SSHWorkspaceProxyPcAppsValues derives the pc-applications values for the SSH workspace proxy
// service from the codesphere.sshWorkspaceProxy.service block. They are the base of the pc-apps
// values: an explicit `pcApps` block in config.yaml and any --pc-apps-values file still override
// them. Returns nil when the config has no service block, leaving the chart defaults untouched.
func SSHWorkspaceProxyPcAppsValues(config *files.RootConfig) files.ChartValues {
	service := config.Codesphere.SSHWorkspaceProxy.ServiceConfig()
	if service == nil {
		return nil
	}

	serviceValues := files.ChartValues{"type": service.ServiceType()}
	if service.UsesNodePort() {
		serviceValues["nodePort"] = service.ServiceNodePort()
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"ssh-workspace-proxy": files.ChartValues{
				"valuesObject": files.ChartValues{
					"service": serviceValues,
				},
			},
		},
	}
}
