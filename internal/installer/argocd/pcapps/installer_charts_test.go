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

type (
	m = map[string]any
	l = []any
)

var _ = Describe("InstallerChartsPcAppsValues", func() {
	allCharts := []string{pcapps.MsBackendS3, pcapps.Rgw, pcapps.RgwLoadBalancer}

	ceph := func() m { return m{"hosts": l{m{"ipAddress": "10.0.0.11"}}} }
	withRgw := func() m { return m{"ceph": ceph(), "cluster": m{"rgw": m{}}} }

	valuesOf := func(config m, skipped ...string) files.ChartValues {
		return pcapps.InstallerChartsPcAppsValues(config, func(step string) bool { return slices.Contains(skipped, step) })
	}
	application := func(config m, chart string) files.ChartValues {
		applications, _ := valuesOf(config)["applications"].(files.ChartValues)
		app, _ := applications[chart].(files.ChartValues)

		return app
	}
	valuesObject := func(config m, chart string) any {
		return application(config, chart)["valuesObject"]
	}
	// turnedOff lists the charts the values switch off with `enabled: false`.
	turnedOff := func(config m, skipped ...string) []string {
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
	writesEnabledTrue := func(config m) bool {
		applications, _ := valuesOf(config)["applications"].(files.ChartValues)

		return slices.ContainsFunc(slices.Collect(maps.Values(applications)), func(app any) bool {
			return app.(files.ChartValues)["enabled"] == true
		})
	}
	// wanted lists the charts the values leave to the pc-applications default.
	wanted := func(config m, skipped ...string) []string {
		off := turnedOff(config, skipped...)

		return slices.DeleteFunc(slices.Clone(allCharts), func(chart string) bool { return slices.Contains(off, chart) })
	}

	Describe("which charts the config asks for", func() {
		It("asks for rgw, rgw-load-balancer and ms-backend-s3 with ceph and an rgw", func() {
			Expect(turnedOff(withRgw())).To(BeEmpty())
		})

		It("asks for rgw only with ceph and an enabled cluster.rgw", func() {
			noCeph := m{"cluster": m{"rgw": m{}}}
			disabled := m{"ceph": ceph(), "cluster": m{"rgw": m{"enabled": false}}}

			Expect(wanted(noCeph)).ToNot(ContainElement(pcapps.Rgw))
			Expect(wanted(disabled)).ToNot(ContainElement(pcapps.Rgw))
			Expect(wanted(m{"ceph": ceph()})).ToNot(ContainElement(pcapps.Rgw))
		})

		It("asks for the load balancer with ceph unless the user disabled it", func() {
			disabled := m{"ceph": ceph(), "cluster": m{"rgwLoadBalancer": m{"enabled": false, "override": m{"replicas": 1}}}}

			Expect(wanted(m{"ceph": ceph()})).To(ContainElement(pcapps.RgwLoadBalancer))
			Expect(wanted(m{})).ToNot(ContainElement(pcapps.RgwLoadBalancer))
			Expect(wanted(disabled)).ToNot(ContainElement(pcapps.RgwLoadBalancer))
		})

		It("asks for ms-backend-s3 by default", func() {
			Expect(wanted(m{})).To(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 when it is disabled", func() {
			disabled := m{"managedServiceBackends": m{"s3": m{"enabled": false, "override": m{"replicas": 1}}}}

			Expect(wanted(disabled)).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 with workspace PVC storage in codesphere.experiments and no ceph", func() {
			Expect(wanted(m{"codesphere": m{"experiments": l{"workspace-pvc-storage"}}})).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("does not ask for ms-backend-s3 with workspace PVC storage in codesphere.internal, no ceph and no s3 block", func() {
			Expect(wanted(m{"codesphere": m{"internal": l{"workspace-pvc-storage"}}})).ToNot(ContainElement(pcapps.MsBackendS3))
		})

		It("asks for ms-backend-s3 with workspace PVC storage and ceph", func() {
			Expect(wanted(m{"codesphere": m{"internal": l{"workspace-pvc-storage"}}, "ceph": ceph()})).To(ContainElement(pcapps.MsBackendS3))
		})

		It("asks for ms-backend-s3 with workspace PVC storage and an explicit s3 block", func() {
			explicit := m{"codesphere": m{"internal": l{"workspace-pvc-storage"}}, "managedServiceBackends": m{"s3": m{}}}

			Expect(wanted(explicit)).To(ContainElement(pcapps.MsBackendS3))
		})

		It("takes an explicit enabled: true of the config as asked for", func() {
			config := m{
				"ceph":                   ceph(),
				"cluster":                m{"rgw": m{"enabled": true}, "rgwLoadBalancer": m{"enabled": true}},
				"managedServiceBackends": m{"s3": m{"enabled": true}},
			}

			Expect(turnedOff(config)).To(BeEmpty())
		})
	})

	Describe("what the values say about a chart", func() {
		It("turns off a chart the config does not ask for", func() {
			Expect(application(m{}, pcapps.Rgw)).To(Equal(files.ChartValues{"enabled": false}))
			Expect(turnedOff(m{})).To(Equal([]string{pcapps.Rgw, pcapps.RgwLoadBalancer}))
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
			Expect(writesEnabledTrue(m{})).To(BeFalse())
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
			config := m{"ceph": m{"hosts": l{m{"ipAddress": "10.0.0.11"}, m{"ipAddress": "10.0.0.12"}}}}

			Expect(valuesObject(config, pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.0.0.11"}, map[string]any{"ip": "10.0.0.12"}},
			}))
		})

		It("forwards the load balancer to the ceph hosts when cluster.rgw is disabled", func() {
			config := m{"ceph": ceph(), "cluster": m{"rgw": m{"enabled": false}}}

			Expect(valuesObject(config, pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.0.0.11"}},
			}))
		})

		It("merges the override of the block over the derived values", func() {
			config := m{"ceph": ceph(), "cluster": m{"rgwLoadBalancer": m{"override": m{"replicas": 3, "externalRgwEndpoints": l{m{"ip": "10.9.9.9"}}}}}}

			Expect(valuesObject(config, pcapps.RgwLoadBalancer)).To(Equal(files.ChartValues{
				"replicas":             3,
				"externalRgwEndpoints": []any{map[string]any{"ip": "10.9.9.9"}},
			}))
		})

		It("passes the override of rgw", func() {
			withOverride := m{"ceph": ceph(), "cluster": m{"rgw": m{"override": m{"resources": m{"requests": m{"cpu": "0"}}}}}}

			Expect(application(withOverride, pcapps.Rgw)).To(Equal(files.ChartValues{
				"valuesObject": files.ChartValues{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			}))
		})

		It("reads the ms-backend-s3 admin credentials from the vault entries a private cloud has", func() {
			Expect(valuesObject(m{}, pcapps.MsBackendS3)).To(Equal(files.ChartValues{
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
			config := m{"managedServiceBackends": m{"s3": m{"override": m{"replicas": 1}}}}

			Expect(valuesObject(config, pcapps.MsBackendS3)).To(HaveKeyWithValue("replicas", 1))
			Expect(valuesObject(config, pcapps.MsBackendS3)).To(HaveKey("extraSecrets"))
		})
	})
})
