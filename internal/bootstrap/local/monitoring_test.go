// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("disableLokiMonitoring", func() {
	It("disables Loki, Grafana and Alloy on an empty config", func() {
		config := &files.RootConfig{}

		disableLokiMonitoring(config)

		Expect(config.Cluster.Monitoring).NotTo(BeNil())
		Expect(config.Cluster.Monitoring.Loki.Enabled).To(BeFalse())
		Expect(config.Cluster.Monitoring.Grafana.Enabled).To(BeFalse())
		Expect(config.Cluster.Monitoring.GrafanaAlloy.Enabled).To(BeFalse())
	})

	It("keeps components that the config already sets", func() {
		config := &files.RootConfig{
			Cluster: files.ClusterConfig{
				Monitoring: &files.MonitoringConfig{
					Loki:    &files.LokiConfig{Enabled: true},
					Grafana: &files.GrafanaConfig{Enabled: true},
				},
			},
		}

		disableLokiMonitoring(config)

		Expect(config.Cluster.Monitoring.Loki.Enabled).To(BeTrue())
		Expect(config.Cluster.Monitoring.Grafana.Enabled).To(BeTrue())
		Expect(config.Cluster.Monitoring.GrafanaAlloy.Enabled).To(BeFalse())
	})
})
