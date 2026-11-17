// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// rgwInstalled reports whether the config deploys the opt-in standalone rgw. Mirrors setUpCluster in
// codesphere-monorepo packages/script-lib/src/ts/private-cloud/cluster.ts.
func rgwInstalled(config *files.RootConfig) bool {
	rgw := config.Cluster.Rgw

	return hasCeph(config) && rgw != nil && enabled(rgw.Enabled)
}

// rgwValues is the override of cluster.rgw; the chart reads its credentials from its ExternalSecret.
func rgwValues(config *files.RootConfig) files.ChartValues {
	if rgw := config.Cluster.Rgw; rgw != nil {
		return util.DeepMergeMaps(nil, rgw.Override)
	}

	return nil
}
