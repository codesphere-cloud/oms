// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import "github.com/codesphere-cloud/oms/internal/installer/files"

const (
	rookCephNamespace    = "rook-ceph"
	inClusterRgwEndpoint = Rgw + "." + rookCephNamespace + ".svc.cluster.local"
)

var rgwLoadBalancerBlock = []string{"cluster", "rgwLoadBalancer"}

func rgwLoadBalancerInstalled(config installerConfig) bool {
	return config.has(cephBlock) && config.componentEnabled(rgwLoadBalancerBlock...)
}

// rgwLoadBalancerValues forwards to the in-cluster rgw when the config deploys it, otherwise to the
// ceph hosts. It asks the config like the installer does, not whether rgw is enabled in
// pc-applications: the same rgw serves the endpoint whichever of the two deploys it.
func rgwLoadBalancerValues(config installerConfig) files.ChartValues {
	endpoints := []any{}

	if rgwInstalled(config) {
		endpoints = append(endpoints, map[string]any{"ip": inClusterRgwEndpoint})
	} else {
		hosts, _ := at[[]any](config, cephBlock, "hosts")

		for _, host := range hosts {
			if entry, ok := host.(map[string]any); ok {
				endpoints = append(endpoints, map[string]any{"ip": entry["ipAddress"]})
			}
		}
	}

	return files.ChartValues{"externalRgwEndpoints": endpoints}
}
