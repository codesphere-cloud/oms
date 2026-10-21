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
	It("gateways without Gateway API settings -> enabled and used", func() {
		gateways := cluster.GatewayAPIs(files.ClusterConfig{})

		Expect(gateways).To(Equal(cluster.Gateways{
			Codesphere: cluster.GatewayAPI{Enabled: true, Use: true},
			Workspaces: cluster.GatewayAPI{Enabled: true, Use: true},
		}))
	})

	It("disabled Gateway APIs -> disabled, use defaults to true", func() {
		use := false
		gateways := cluster.GatewayAPIs(files.ClusterConfig{
			Gateway: files.GatewayConfig{GatewayAPI: &files.GatewayAPIConfig{Enabled: false}},
			PublicGateway: files.GatewayConfig{
				GatewayAPI: &files.GatewayAPIConfig{Enabled: false, Use: &use},
			},
		})

		Expect(gateways).To(Equal(cluster.Gateways{
			Codesphere: cluster.GatewayAPI{Enabled: false, Use: true},
			Workspaces: cluster.GatewayAPI{Enabled: false, Use: false},
		}))
	})
})
