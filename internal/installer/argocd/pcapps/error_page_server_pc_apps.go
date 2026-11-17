// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// ErrorPageServerPcAppsValues derives the pc-applications values for error-page-server from the config.yaml.
// The chart reads the Codesphere globals, as it did as a subchart of the umbrella chart,
// and its defaults fit a private cloud except for these three.
// They are the base of the pc-apps values: an explicit `pcApps` block in config.yaml and any --pc-apps-values file
// still override them.
// The pc-applications chart decides whether the application is enabled,
// in step with the umbrella chart dropping its error-page-server.
func ErrorPageServerPcAppsValues(config *files.RootConfig) files.ChartValues {
	return files.ChartValues{
		"applications": files.ChartValues{
			"error-page-server": files.ChartValues{
				"valuesObject": files.ChartValues{
					"global": files.ChartValues{
						// The error pages link back to the IDE on the first host.
						"hosts": []string{config.Codesphere.Domain},
						"customDomains": files.ChartValues{
							"gatewayApi": files.ChartValues{
								// Without the Gateway API, custom domains reach the server through the chart's proxy.
								"useGateway": csappsvalues.WorkspacesUseGatewayAPI(config.Cluster),
							},
						},
						"services": files.ChartValues{
							"priorityClass": csappsvalues.PriorityClass,
						},
					},
				},
			},
		},
	}
}
