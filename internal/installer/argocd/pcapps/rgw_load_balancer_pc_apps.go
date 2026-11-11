// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	rookCephNamespace    = "rook-ceph"
	inClusterRgwEndpoint = Rgw + "." + rookCephNamespace + ".svc.cluster.local"
)

func rgwLoadBalancerInstalled(config *files.RootConfig) bool {
	loadBalancer := config.Cluster.RgwLoadBalancer

	return hasCeph(config) && (loadBalancer == nil || enabled(loadBalancer.Enabled))
}

// rgwLoadBalancerValues points the load balancer at the in-cluster rgw when the config deploys it,
// otherwise at the ceph hosts.
func rgwLoadBalancerValues(config *files.RootConfig) files.ChartValues {
	endpoints := []any{}

	if rgwInstalled(config) {
		endpoints = append(endpoints, map[string]any{"ip": inClusterRgwEndpoint})
	} else {
		for _, host := range config.Ceph.Hosts {
			endpoints = append(endpoints, map[string]any{"ip": host.IPAddress})
		}
	}

	values := files.ChartValues{"externalRgwEndpoints": endpoints}
	if loadBalancer := config.Cluster.RgwLoadBalancer; loadBalancer != nil {
		values = util.DeepMergeMaps(values, loadBalancer.Override)
	}

	return values
}
