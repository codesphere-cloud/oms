// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("ErrorPageServerPcAppsValues", func() {
	config := func() *files.RootConfig {
		config := files.NewRootConfig()
		config.Codesphere.Domain = "codesphere.example.com"

		return &config
	}

	// The error-page-server application in the rendered values.
	application := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		app, ok := apps["error-page-server"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an error-page-server application")

		return app
	}

	global := func(values files.ChartValues) files.ChartValues {
		valuesObject, ok := application(values)["valuesObject"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a valuesObject")

		global, ok := valuesObject["global"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a global block")

		return global
	}

	useGateway := func(values files.ChartValues) any {
		return global(values)["customDomains"].(files.ChartValues)["gatewayApi"].(files.ChartValues)["useGateway"]
	}

	It("passes the domain, the Gateway API use and the services' priority class", func() {
		values := pcapps.ErrorPageServerPcAppsValues(config())

		Expect(global(values)).To(HaveKeyWithValue("hosts", []string{"codesphere.example.com"}))
		Expect(useGateway(values)).To(BeTrue())
		Expect(global(values)["services"]).To(Equal(files.ChartValues{"priorityClass": "prod-codesphere-deployment"}))
	})

	It("leaves enabled to the pc-applications chart", func() {
		Expect(application(pcapps.ErrorPageServerPcAppsValues(config()))).NotTo(HaveKey("enabled"))
	})

	It("public gateway doesn't use the Gateway API -> the chart's proxy", func() {
		cfg := config()
		cfg.Cluster.PublicGateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: true, Use: ptr.To(false)}

		Expect(useGateway(pcapps.ErrorPageServerPcAppsValues(cfg))).To(BeFalse())
	})

	It("public gateway without the Gateway API -> the chart's proxy", func() {
		cfg := config()
		cfg.Cluster.PublicGateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}

		Expect(useGateway(pcapps.ErrorPageServerPcAppsValues(cfg))).To(BeFalse())
	})
})
