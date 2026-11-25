// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	// RgwLoadBalancer is the key of the rgw-load-balancer chart under `applications` in the pc-applications values.
	RgwLoadBalancer = "rgw-load-balancer"

	rookCephNamespace    = "rook-ceph"
	inClusterRgwEndpoint = Rgw + "." + rookCephNamespace + ".svc.cluster.local"
)

// RgwLoadBalancerPcAppsValues derives the pc-applications values for the rgw load balancer:
// `enabled: false` when the config does not ask for it or the run skips the set-up-cluster step,
// otherwise the endpoints it forwards to with the override of cluster.rgwLoadBalancer on top.
func RgwLoadBalancerPcAppsValues(config *files.RootConfig, skipSteps []string) files.ChartValues {
	installed := stepRuns(config, skipSteps, installer.SetUpClusterStep) && rgwLoadBalancerInstalled(config)

	return applicationValues(RgwLoadBalancer, installed, rgwLoadBalancerValues(config))
}

// rgwLoadBalancerInstalled mirrors setUpCluster in codesphere-monorepo
// packages/script-lib/src/ts/private-cloud/cluster.ts.
func rgwLoadBalancerInstalled(config *files.RootConfig) bool {
	loadBalancer := config.Cluster.RgwLoadBalancer

	return hasCeph(config) && (loadBalancer == nil || enabled(loadBalancer.Enabled))
}

// rgwLoadBalancerValues points the load balancer at the in-cluster rgw when the config deploys it,
// otherwise at the ceph hosts, as RgwLoadBalancerComponent in the same file does. The chart writes
// each `ip` into an nginx upstream `server`, which also takes a hostname.
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
