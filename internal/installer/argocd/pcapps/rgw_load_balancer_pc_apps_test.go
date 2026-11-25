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

var _ = Describe("rgw-load-balancer application", func() {
	application := func(config *files.RootConfig, skipSteps ...string) files.ChartValues {
		return applicationOf(pcapps.RgwLoadBalancerPcAppsValues(config, skipSteps), pcapps.RgwLoadBalancer)
	}
	valuesObject := func(config *files.RootConfig) any {
		return application(config)["valuesObject"]
	}
	withLoadBalancer := func(loadBalancer *files.RgwLoadBalancerConfig) *files.RootConfig {
		return &files.RootConfig{Ceph: cephWithHosts(), Cluster: files.ClusterConfig{RgwLoadBalancer: loadBalancer}}
	}
	off := files.ChartValues{"enabled": false}

	Describe("whether the config asks for it", func() {
		It("asks for the load balancer with ceph", func() {
			Expect(application(&files.RootConfig{Ceph: cephWithHosts()})).ToNot(Equal(off))
		})

		It("asks for the load balancer with an rgwLoadBalancer block that only overrides values", func() {
			config := withLoadBalancer(&files.RgwLoadBalancerConfig{Override: files.ChartOverride{"replicas": 1}})

			Expect(application(config)).ToNot(Equal(off))
		})

		It("takes an explicit enabled: true of the config as asked for", func() {
			Expect(application(withLoadBalancer(&files.RgwLoadBalancerConfig{Enabled: new(true)}))).ToNot(Equal(off))
		})

		It("turns the application off without ceph", func() {
			Expect(application(&files.RootConfig{})).To(Equal(off))
		})

		It("turns the application off when the user disabled it", func() {
			config := withLoadBalancer(&files.RgwLoadBalancerConfig{Enabled: new(false), Override: files.ChartOverride{"replicas": 1}})

			Expect(application(config)).To(Equal(off))
		})

		It("turns the application off with a ceph block without hosts", func() {
			config := &files.RootConfig{Ceph: files.CephConfig{NodesSubnet: "10.0.0.0/24"}}

			Expect(application(config)).To(Equal(off))
		})

		It("turns the application off when the run skips the set-up-cluster step", func() {
			Expect(application(&files.RootConfig{Ceph: cephWithHosts()}, installer.SetUpClusterStep)).To(Equal(off))
		})

		It("keeps the application when the run skips the ms-backends step", func() {
			Expect(application(&files.RootConfig{Ceph: cephWithHosts()}, installer.MsBackendsStep)).ToNot(Equal(off))
		})
	})

	Describe("the values", func() {
		It("leaves the enablement to the pc-applications default", func() {
			Expect(application(&files.RootConfig{Ceph: cephWithHosts()})).ToNot(HaveKey("enabled"))
		})

		It("forwards the load balancer to the in-cluster rgw when the config deploys it", func() {
			config := withLoadBalancer(nil)
			config.Cluster.Rgw = &files.RgwConfig{}

			Expect(valuesObject(config)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "rgw.rook-ceph.svc.cluster.local"}},
			}))
		})

		It("forwards the load balancer to the ceph hosts otherwise", func() {
			config := &files.RootConfig{Ceph: cephWithHosts("10.0.0.11", "10.0.0.12")}

			Expect(valuesObject(config)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.0.0.11"}, map[string]any{"ip": "10.0.0.12"}},
			}))
		})

		It("forwards the load balancer to the ceph hosts when cluster.rgw is disabled", func() {
			config := &files.RootConfig{Ceph: cephWithHosts(), Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{Enabled: new(false)}}}

			Expect(valuesObject(config)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.0.0.11"}},
			}))
		})

		It("merges the override of the load balancer over the derived values", func() {
			config := withLoadBalancer(&files.RgwLoadBalancerConfig{
				Override: files.ChartOverride{"replicas": 3, "externalRgwEndpoints": []any{map[string]any{"ip": "10.9.9.9"}}},
			})

			Expect(valuesObject(config)).To(Equal(files.ChartValues{
				"replicas":             3,
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.9.9.9"}},
			}))
		})
	})
})
