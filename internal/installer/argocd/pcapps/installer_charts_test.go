// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"maps"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InstallerChartsPcAppsValues", func() {
	allCharts := []string{pcapps.MsBackendS3, pcapps.Rgw, pcapps.RgwLoadBalancer}

	ceph := func(ips ...string) files.CephConfig {
		if len(ips) == 0 {
			ips = []string{"10.0.0.11"}
		}

		hosts := make([]files.CephHost, 0, len(ips))
		for _, ip := range ips {
			hosts = append(hosts, files.CephHost{IPAddress: ip})
		}

		return files.CephConfig{Hosts: hosts}
	}
	withRgw := func() *files.RootConfig {
		return &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{}}}
	}
	withS3 := func(s3 *files.S3ManagedServiceConfig) *files.ManagedServiceBackendsConfig {
		return &files.ManagedServiceBackendsConfig{S3: s3}
	}

	valuesOf := func(config *files.RootConfig, skipped ...string) files.ChartValues {
		return pcapps.InstallerChartsPcAppsValues(config, skipped)
	}
	application := func(config *files.RootConfig, chart string) files.ChartValues {
		applications, _ := valuesOf(config)["applications"].(files.ChartValues)
		app, _ := applications[chart].(files.ChartValues)

		return app
	}
	valuesObject := func(config *files.RootConfig, chart string) any {
		return application(config, chart)["valuesObject"]
	}
	// turnedOff lists the charts the values switch off with `enabled: false`.
	turnedOff := func(config *files.RootConfig, skipped ...string) []string {
		applications, _ := valuesOf(config, skipped...)["applications"].(files.ChartValues)
		off := []string{}

		for name, app := range applications {
			if app.(files.ChartValues)["enabled"] == false {
				off = append(off, name)
			}
		}

		slices.Sort(off)

		return off
	}
	writesEnabledTrue := func(config *files.RootConfig) bool {
		applications, _ := valuesOf(config)["applications"].(files.ChartValues)

		return slices.ContainsFunc(slices.Collect(maps.Values(applications)), func(app any) bool {
			return app.(files.ChartValues)["enabled"] == true
		})
	}
	// wanted lists the charts the values leave to the pc-applications default.
	wanted := func(config *files.RootConfig, skipped ...string) []string {
		off := turnedOff(config, skipped...)

		return slices.DeleteFunc(slices.Clone(allCharts), func(chart string) bool { return slices.Contains(off, chart) })
	}

	Describe("which charts the config asks for", func() {
		It("asks for rgw, rgw-load-balancer and ms-backend-s3 with ceph and an rgw", func() {
			Expect(turnedOff(withRgw())).To(BeEmpty())
		})

		It("asks for rgw only with ceph and an enabled cluster.rgw", func() {
			noCeph := &files.RootConfig{Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{}}}
			disabled := &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{Enabled: new(false)}}}

			Expect(wanted(noCeph)).ToNot(ContainElement(pcapps.Rgw))
			Expect(wanted(disabled)).ToNot(ContainElement(pcapps.Rgw))
			Expect(wanted(&files.RootConfig{Ceph: ceph()})).ToNot(ContainElement(pcapps.Rgw))
		})

		It("asks for the load balancer with ceph unless the user disabled it", func() {
			disabled := &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{RgwLoadBalancer: &files.RgwLoadBalancerConfig{
				Enabled: new(false), Override: files.ChartOverride{"replicas": 1},
			}}}

			Expect(wanted(&files.RootConfig{Ceph: ceph()})).To(ContainElement(pcapps.RgwLoadBalancer))
			Expect(wanted(&files.RootConfig{})).ToNot(ContainElement(pcapps.RgwLoadBalancer))
			Expect(wanted(disabled)).ToNot(ContainElement(pcapps.RgwLoadBalancer))
		})

		It("asks for the load balancer with an rgwLoadBalancer block that only overrides values", func() {
			overrideOnly := &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{RgwLoadBalancer: &files.RgwLoadBalancerConfig{
				Override: files.ChartOverride{"replicas": 1},
			}}}

			Expect(wanted(overrideOnly)).To(ContainElement(pcapps.RgwLoadBalancer))
		})

		It("does not ask for rgw or its load balancer with a ceph block without hosts", func() {
			config := &files.RootConfig{Ceph: files.CephConfig{NodesSubnet: "10.0.0.0/24"}, Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{}}}

			Expect(wanted(config)).ToNot(ContainElements(pcapps.Rgw, pcapps.RgwLoadBalancer))
		})

		It("asks for ms-backend-s3 by default", func() {
			Expect(wanted(&files.RootConfig{})).To(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 when it is disabled", func() {
			disabled := &files.RootConfig{ManagedServiceBackends: withS3(&files.S3ManagedServiceConfig{
				Enabled: new(false), Override: files.ChartOverride{"replicas": 1},
			})}

			Expect(wanted(disabled)).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 with workspace PVC storage in codesphere.experiments and no ceph", func() {
			config := &files.RootConfig{Codesphere: files.CodesphereConfig{Experiments: []string{files.WorkspacePVCStorageFlag}}} //nolint:staticcheck

			Expect(wanted(config)).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 with workspace PVC storage in codesphere.internal, no ceph and no s3 block", func() {
			config := &files.RootConfig{Codesphere: files.CodesphereConfig{Internal: []string{files.WorkspacePVCStorageFlag}}}

			Expect(wanted(config)).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 with workspace PVC storage as a preview flag and no ceph", func() {
			config := &files.RootConfig{Codesphere: files.CodesphereConfig{Preview: map[string]bool{files.WorkspacePVCStorageFlag: true}}}

			Expect(wanted(config)).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("asks for ms-backend-s3 with workspace PVC storage and ceph", func() {
			config := &files.RootConfig{Ceph: ceph(), Codesphere: files.CodesphereConfig{Internal: []string{files.WorkspacePVCStorageFlag}}}

			Expect(wanted(config)).To(ContainElement(pcapps.MsBackendS3))
		})

		It("asks for ms-backend-s3 with workspace PVC storage and an explicit s3 block", func() {
			explicit := &files.RootConfig{
				Codesphere:             files.CodesphereConfig{Internal: []string{files.WorkspacePVCStorageFlag}},
				ManagedServiceBackends: withS3(&files.S3ManagedServiceConfig{}),
			}

			Expect(wanted(explicit)).To(ContainElement(pcapps.MsBackendS3))
		})

		It("takes an explicit enabled: true of the config as asked for", func() {
			config := &files.RootConfig{
				Ceph: ceph(),
				Cluster: files.ClusterConfig{
					Rgw:             &files.RgwConfig{Enabled: new(true)},
					RgwLoadBalancer: &files.RgwLoadBalancerConfig{Enabled: new(true)},
				},
				ManagedServiceBackends: withS3(&files.S3ManagedServiceConfig{Enabled: new(true)}),
			}

			Expect(turnedOff(config)).To(BeEmpty())
		})
	})

	Describe("what the values say about a chart", func() {
		It("turns off a chart the config does not ask for", func() {
			Expect(application(&files.RootConfig{}, pcapps.Rgw)).To(Equal(files.ChartValues{"enabled": false}))
			Expect(turnedOff(&files.RootConfig{})).To(Equal([]string{pcapps.Rgw, pcapps.RgwLoadBalancer}))
		})

		It("turns off the charts of a step the run skips", func() {
			Expect(turnedOff(withRgw(), installer.SetUpClusterStep)).To(Equal([]string{pcapps.Rgw, pcapps.RgwLoadBalancer}))
			Expect(turnedOff(withRgw(), installer.MsBackendsStep)).To(Equal([]string{pcapps.MsBackendS3}))
		})

		It("leaves the enablement of a chart the config asks for to the pc-applications default", func() {
			Expect(application(withRgw(), pcapps.RgwLoadBalancer)).ToNot(HaveKey("enabled"))
			Expect(application(withRgw(), pcapps.MsBackendS3)).ToNot(HaveKey("enabled"))
		})

		It("never writes enabled: true for a chart the config asks for", func() {
			Expect(writesEnabledTrue(withRgw())).To(BeFalse())
		})

		It("never writes enabled: true for a chart the config does not ask for", func() {
			Expect(writesEnabledTrue(&files.RootConfig{})).To(BeFalse())
		})

		It("writes nothing for a chart the config asks for and that has no values", func() {
			Expect(valuesOf(withRgw())["applications"]).ToNot(HaveKey(pcapps.Rgw))
			Expect(application(withRgw(), pcapps.Rgw)).To(BeEmpty())
		})
	})

	Describe("the values", func() {
		It("forwards the load balancer to the in-cluster rgw when the config deploys it", func() {
			Expect(valuesObject(withRgw(), pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "rgw.rook-ceph.svc.cluster.local"}},
			}))
		})

		It("forwards the load balancer to the ceph hosts otherwise", func() {
			config := &files.RootConfig{Ceph: ceph("10.0.0.11", "10.0.0.12")}

			Expect(valuesObject(config, pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.0.0.11"}, map[string]any{"ip": "10.0.0.12"}},
			}))
		})

		It("forwards the load balancer to the ceph hosts when cluster.rgw is disabled", func() {
			config := &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{Enabled: new(false)}}}

			Expect(valuesObject(config, pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.0.0.11"}},
			}))
		})

		It("merges the override of the load balancer over the derived values", func() {
			config := &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{RgwLoadBalancer: &files.RgwLoadBalancerConfig{
				Override: files.ChartOverride{"replicas": 3, "externalRgwEndpoints": []any{map[string]any{"ip": "10.9.9.9"}}},
			}}}

			Expect(valuesObject(config, pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"replicas":             3,
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.9.9.9"}},
			}))
		})

		It("passes the override of rgw", func() {
			withOverride := &files.RootConfig{Ceph: ceph(), Cluster: files.ClusterConfig{Rgw: &files.RgwConfig{
				Override: files.ChartOverride{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			}}}

			Expect(application(withOverride, pcapps.Rgw)).To(Equal(files.ChartValues{
				"valuesObject": files.ChartValues{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			}))
		})

		It("reads the ms-backend-s3 admin credentials from the vault entries a private cloud has", func() {
			Expect(valuesObject(&files.RootConfig{}, pcapps.MsBackendS3)).To(Equal(files.ChartValues{
				"extraSecrets": []any{map[string]any{
					"name":  "app-secrets",
					"store": "kubernetes-in-cluster",
					"secretKeys": []any{
						map[string]any{"secretKey": "RGW_ADMIN_ACCESS_KEY_ID", "remoteRef": "cs-vault", "property": "rgwAdminAccessKey.password"},
						map[string]any{"secretKey": "RGW_ADMIN_SECRET_ACCESS_KEY", "remoteRef": "cs-vault", "property": "rgwAdminSecretKey.password"},
					},
				}},
			}))
		})

		It("lets the override of ms-backend-s3 add to its secrets", func() {
			config := &files.RootConfig{ManagedServiceBackends: withS3(&files.S3ManagedServiceConfig{Override: files.ChartOverride{"replicas": 1}})}

			Expect(valuesObject(config, pcapps.MsBackendS3)).To(HaveKeyWithValue("replicas", 1))
			Expect(valuesObject(config, pcapps.MsBackendS3)).To(HaveKey("extraSecrets"))
		})
	})
})
