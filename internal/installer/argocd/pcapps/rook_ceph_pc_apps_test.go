// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("RookCephPcAppsValues", func() {
	// The csi block of the rook-ceph application in the rendered values.
	csiValues := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		rookCeph, ok := apps["rook-ceph"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a rook-ceph application")

		csi, ok := rookCeph["csi"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a csi block")

		return csi
	}

	It("returns nil when ceph.csiKubeletDir is not set", func() {
		inputConfig := &files.RootConfig{
			PcApps: files.ChartValues{
				"applications": files.ChartValues{
					"rook-ceph": files.ChartValues{},
				},
			},
		}

		values := pcapps.RookCephPcAppsValues(inputConfig)

		Expect(values).To(BeNil())
	})

	It("passes ceph.csiKubeletDir to the rook-ceph application", func() {
		inputConfig := &files.RootConfig{
			Ceph: files.CephConfig{
				CsiKubeletDir: "/var/lib/k0s/kubelet",
			},
			PcApps: files.ChartValues{
				"applications": files.ChartValues{
					"rook-ceph": files.ChartValues{},
				},
			},
		}

		values := pcapps.RookCephPcAppsValues(inputConfig)

		Expect(csiValues(values)).To(Equal(files.ChartValues{"kubeletDirPath": "/var/lib/k0s/kubelet"}))
	})

	Context("when the pcApps block also configures rook-ceph", func() {
		It("lets the pcApps kubeletDirPath win over ceph.csiKubeletDir", func() {
			inputConfig := &files.RootConfig{
				Ceph: files.CephConfig{
					CsiKubeletDir: "/var/lib/k0s/kubelet",
				},
				PcApps: files.ChartValues{
					"applications": files.ChartValues{
						"rook-ceph": files.ChartValues{
							"csi": files.ChartValues{"kubeletDirPath": "/custom/kubelet"},
						},
					},
				},
			}

			values := util.DeepMergeMaps(pcapps.RookCephPcAppsValues(inputConfig), inputConfig.PcApps)

			Expect(csiValues(values)).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})

		It("applies the pcApps kubeletDirPath without ceph.csiKubeletDir", func() {
			inputConfig := &files.RootConfig{
				PcApps: files.ChartValues{
					"applications": files.ChartValues{
						"rook-ceph": files.ChartValues{
							"csi": files.ChartValues{"kubeletDirPath": "/custom/kubelet"},
						},
					},
				},
			}

			values := util.DeepMergeMaps(pcapps.RookCephPcAppsValues(inputConfig), inputConfig.PcApps)

			Expect(csiValues(values)).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})
	})
})
