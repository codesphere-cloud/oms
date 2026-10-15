// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("RookCephPcAppsValues", func() {
	configWith := func(kubeletDir string) *files.RootConfig {
		config := &files.RootConfig{}
		config.Ceph.CsiKubeletDir = kubeletDir

		return config
	}

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

	// Merges the derived values with the pcApps block the same way the installer does:
	// the pcApps block from config.yaml is applied on top.
	mergeWithPcApps := func(config *files.RootConfig) files.ChartValues {
		return util.DeepMergeMaps(pcapps.RookCephPcAppsValues(config), config.PcApps)
	}

	It("leaves the chart default alone when ceph.csiKubeletDir is not set", func() {
		Expect(pcapps.RookCephPcAppsValues(configWith(""))).To(BeNil())
	})

	It("passes ceph.csiKubeletDir to the rook-ceph application", func() {
		values := pcapps.RookCephPcAppsValues(configWith("/var/lib/k0s/kubelet"))

		Expect(csiValues(values)).To(Equal(files.ChartValues{"kubeletDirPath": "/var/lib/k0s/kubelet"}))
	})

	Context("when the pcApps block also configures rook-ceph", func() {
		It("lets the pcApps kubeletDirPath win over ceph.csiKubeletDir", func() {
			config := configWith("/var/lib/k0s/kubelet")
			config.PcApps = files.ChartValues{
				"applications": files.ChartValues{
					"rook-ceph": files.ChartValues{
						"csi": files.ChartValues{"kubeletDirPath": "/custom/kubelet"},
					},
				},
			}

			Expect(csiValues(mergeWithPcApps(config))).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})

		It("keeps ceph.csiKubeletDir when pcApps only sets other csi values", func() {
			config := configWith("/var/lib/k0s/kubelet")
			config.PcApps = files.ChartValues{
				"applications": files.ChartValues{
					"rook-ceph": files.ChartValues{
						"csi": files.ChartValues{"enableRbdDriver": false},
					},
				},
			}

			Expect(csiValues(mergeWithPcApps(config))).To(Equal(files.ChartValues{
				"kubeletDirPath":  "/var/lib/k0s/kubelet",
				"enableRbdDriver": false,
			}))
		})

		It("applies the pcApps kubeletDirPath even without ceph.csiKubeletDir", func() {
			config := configWith("")
			config.PcApps = files.ChartValues{
				"applications": files.ChartValues{
					"rook-ceph": files.ChartValues{
						"csi": files.ChartValues{"kubeletDirPath": "/custom/kubelet"},
					},
				},
			}

			Expect(csiValues(mergeWithPcApps(config))).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})

		It("lets the pcApps block win when both are read from config.yaml", func() {
			config := &files.RootConfig{}
			Expect(yaml.Unmarshal([]byte(`
ceph:
  csiKubeletDir: /var/lib/k0s/kubelet
pcApps:
  applications:
    rook-ceph:
      csi:
        kubeletDirPath: /custom/kubelet
`), config)).To(Succeed())

			Expect(csiValues(mergeWithPcApps(config))).To(HaveKeyWithValue("kubeletDirPath", "/custom/kubelet"))
		})
	})
})
