// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package cluster reads the cluster settings the Codesphere values depend on.
package cluster

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// GatewayAPI says whether a gateway uses the Gateway API.
type GatewayAPI struct {
	Enabled bool
	Use     bool
}

// Gateways are the Gateway API settings of the cluster's two gateways.
type Gateways struct {
	// Codesphere serves Codesphere itself.
	Codesphere GatewayAPI
	// Workspaces serves workspaces and their custom domains.
	Workspaces GatewayAPI
}

// GatewayAPIs reads the gateways' Gateway API settings, which default to enabled and used.
func GatewayAPIs(cluster files.ClusterConfig) Gateways {
	return Gateways{
		Codesphere: gatewayAPIOf(cluster.Gateway),
		Workspaces: gatewayAPIOf(cluster.PublicGateway),
	}
}

func gatewayAPIOf(conf files.GatewayConfig) GatewayAPI {
	if conf.GatewayAPI == nil {
		return GatewayAPI{Enabled: true, Use: true}
	}

	use := true
	if conf.GatewayAPI.Use != nil {
		use = *conf.GatewayAPI.Use
	}

	return GatewayAPI{Enabled: conf.GatewayAPI.Enabled, Use: use}
}
