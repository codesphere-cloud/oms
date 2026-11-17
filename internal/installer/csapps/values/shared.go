// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values

import (
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/platform"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// PriorityClass is the priority class of the Codesphere services.
// pc-applications gives it to error-page-server, which used to be one of them.
const PriorityClass = platform.PriorityClass

// WorkspacesUseGatewayAPI reports whether workspaces and their custom domains go through the Gateway API,
// as the Codesphere values decide it.
func WorkspacesUseGatewayAPI(conf files.ClusterConfig) bool {
	return cluster.GatewayAPIs(conf).Workspaces.Use
}
