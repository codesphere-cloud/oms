// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("rgw application", func() {
	application := func(config *files.RootConfig, skipSteps ...string) files.ChartValues {
		return applicationOf(pcapps.RgwPcAppsValues(config, skipSteps), pcapps.Rgw)
	}
	withRgw := func(rgw *files.RgwConfig) *files.RootConfig {
		return &files.RootConfig{Ceph: cephWithHosts(), Cluster: files.ClusterConfig{Rgw: rgw}}
	}
	off := files.ChartValues{"enabled": false}

	Describe("whether the config asks for it", func() {
		It("asks for rgw with ceph and a cluster.rgw block", func() {
			Expect(application(withRgw(&files.RgwConfig{}))).ToNot(Equal(off))
		})

		It("takes an explicit enabled: true of the config as asked for", func() {
			Expect(application(withRgw(&files.RgwConfig{Enabled: new(true)}))).ToNot(Equal(off))
		})

		It("turns the application off without ceph", func() {
			Expect(application(&files.RootConfig{Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{}}})).To(Equal(off))
		})

		It("turns the application off without a cluster.rgw block", func() {
			Expect(application(&files.RootConfig{Ceph: cephWithHosts()})).To(Equal(off))
		})

		It("turns the application off when cluster.rgw is disabled", func() {
			Expect(application(withRgw(&files.RgwConfig{Enabled: new(false)}))).To(Equal(off))
		})

		It("turns the application off with a ceph block without hosts", func() {
			config := &files.RootConfig{Ceph: files.CephConfig{NodesSubnet: "10.0.0.0/24"}, Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{}}}

			Expect(application(config)).To(Equal(off))
		})

		It("turns the application off when the run skips the set-up-cluster step", func() {
			Expect(application(withRgw(&files.RgwConfig{}), installer.SetUpClusterStep)).To(Equal(off))
		})

		It("keeps the application when the run skips the ms-backends step", func() {
			Expect(application(withRgw(&files.RgwConfig{}), installer.MsBackendsStep)).ToNot(Equal(off))
		})
	})

	Describe("the values", func() {
		It("writes nothing for an rgw without an override", func() {
			Expect(pcapps.RgwPcAppsValues(withRgw(&files.RgwConfig{}), nil)["applications"]).ToNot(HaveKey(pcapps.Rgw))
		})

		It("never writes enabled: true", func() {
			Expect(application(withRgw(&files.RgwConfig{Enabled: new(true)}))).ToNot(HaveKey("enabled"))
		})

		It("passes the override of rgw", func() {
			config := withRgw(&files.RgwConfig{
				Override: files.ChartOverride{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			})

			Expect(application(config)).To(Equal(files.ChartValues{
				"valuesObject": files.ChartValues{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			}))
		})
	})
})
