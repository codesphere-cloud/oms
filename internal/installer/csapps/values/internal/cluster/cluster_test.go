// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cluster_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("GatewayAPIs", func() {
	workspacesGatewayAPI := func(conf *files.GatewayAPIConfig) cluster.GatewayAPI {
		return cluster.GatewayAPIs(files.ClusterConfig{
			PublicGateway: files.GatewayConfig{GatewayAPI: conf},
		}).Workspaces
	}

	It("gateways without Gateway API settings -> enabled and used", func() {
		gateways := cluster.GatewayAPIs(files.ClusterConfig{})

		Expect(gateways).To(Equal(cluster.Gateways{
			Codesphere: cluster.GatewayAPI{Enabled: true, Use: true},
			Workspaces: cluster.GatewayAPI{Enabled: true, Use: true},
		}))
	})

	It("enabled Gateway API without use -> used", func() {
		gatewayAPI := workspacesGatewayAPI(&files.GatewayAPIConfig{Enabled: true})

		Expect(gatewayAPI).To(Equal(cluster.GatewayAPI{Enabled: true, Use: true}))
	})

	It("enabled Gateway API with use false -> not used", func() {
		use := false
		gatewayAPI := workspacesGatewayAPI(&files.GatewayAPIConfig{Enabled: true, Use: &use})

		Expect(gatewayAPI).To(Equal(cluster.GatewayAPI{Enabled: true, Use: false}))
	})

	It("disabled Gateway API without use -> not used", func() {
		gatewayAPI := workspacesGatewayAPI(&files.GatewayAPIConfig{Enabled: false})

		Expect(gatewayAPI).To(Equal(cluster.GatewayAPI{Enabled: false, Use: false}))
	})

	It("disabled Gateway API with use true -> not used", func() {
		use := true
		gatewayAPI := workspacesGatewayAPI(&files.GatewayAPIConfig{Enabled: false, Use: &use})

		Expect(gatewayAPI).To(Equal(cluster.GatewayAPI{Enabled: false, Use: false}))
	})
})
