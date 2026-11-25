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
	// The csi block of the upstream Rook sub-chart inside the rook-ceph application's chart values.
	csiValues := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		rookCephApp, ok := apps["rook-ceph"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a rook-ceph application")

		valuesObject, ok := rookCephApp["valuesObject"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a valuesObject block")

		rookSubChart, ok := valuesObject["rook-ceph"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a rook-ceph sub-chart block")

		csi, ok := rookSubChart["csi"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a csi block")

		return csi
	}

	// pcApps values that set the CSI kubelet dir the way the workaround in the install config does.
	pcAppsWithKubeletDir := func(kubeletDir string) files.ChartValues {
		return files.ChartValues{
			"applications": files.ChartValues{
				"rook-ceph": files.ChartValues{
					"valuesObject": files.ChartValues{
						"rook-ceph": files.ChartValues{
							"csi": files.ChartValues{"kubeletDirPath": kubeletDir},
						},
					},
				},
			},
		}
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

	It("passes ceph.csiKubeletDir to the upstream Rook sub-chart of the rook-ceph application", func() {
		inputConfig := &files.RootConfig{
			Ceph: files.CephConfig{
				CsiKubeletDir: "/custom/kubelet",
			},
			PcApps: files.ChartValues{
				"applications": files.ChartValues{
					"rook-ceph": files.ChartValues{},
				},
			},
		}

		values := pcapps.RookCephPcAppsValues(inputConfig)

		Expect(csiValues(values)).To(Equal(files.ChartValues{"kubeletDirPath": "/custom/kubelet"}))
	})

	Context("when the pcApps block also configures rook-ceph", func() {
		It("lets the pcApps kubeletDirPath win over ceph.csiKubeletDir", func() {
			inputConfig := &files.RootConfig{
				Ceph: files.CephConfig{
					CsiKubeletDir: "/var/lib/k0s/kubelet",
				},
				PcApps: pcAppsWithKubeletDir("/custom/kubelet"),
			}

			values := util.DeepMergeMaps(pcapps.RookCephPcAppsValues(inputConfig), inputConfig.PcApps)

			Expect(csiValues(values)).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})

		It("applies the pcApps kubeletDirPath without ceph.csiKubeletDir", func() {
			inputConfig := &files.RootConfig{
				PcApps: pcAppsWithKubeletDir("/custom/kubelet"),
			}

			values := util.DeepMergeMaps(pcapps.RookCephPcAppsValues(inputConfig), inputConfig.PcApps)

			Expect(csiValues(values)).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})
	})
})
