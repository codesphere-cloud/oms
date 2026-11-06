// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

var rgwBlock = []string{"cluster", "rgw"}

// rgwInstalled reports whether the config deploys the opt-in standalone rgw. The installer's load
// balancer takes its endpoint from the same answer (inClusterRgw). The chart needs no derived
// values: the daemon credentials come from its ExternalSecret.
func rgwInstalled(config installerConfig) bool {
	return config.has(cephBlock) && config.has(rgwBlock...) && config.componentEnabled(rgwBlock...)
}
