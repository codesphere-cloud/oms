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
	configWith := func(kubeletDir string) *files.RootConfig {
		config := &files.RootConfig{}
		config.Ceph.CsiKubeletDir = kubeletDir

		return config
	}

	kubeletDirPath := func(values files.ChartValues) any {
		apps := values["applications"].(files.ChartValues)
		rookCeph := apps["rook-ceph"].(files.ChartValues)
		valuesObject := rookCeph["valuesObject"].(files.ChartValues)

		return valuesObject["csi"].(files.ChartValues)["kubeletDirPath"]
	}

	It("leaves the chart default alone when ceph.csiKubeletDir is not set", func() {
		Expect(pcapps.RookCephPcAppsValues(configWith(""))).To(BeNil())
	})

	It("passes ceph.csiKubeletDir to the rook-ceph application", func() {
		values := pcapps.RookCephPcAppsValues(configWith("/var/lib/k0s/kubelet"))

		Expect(kubeletDirPath(values)).To(Equal("/var/lib/k0s/kubelet"))
	})

	It("is overridden by a kubeletDirPath configured under pcApps", func() {
		pcApps := files.ChartValues{
			"applications": files.ChartValues{
				"rook-ceph": files.ChartValues{
					"valuesObject": files.ChartValues{
						"csi": files.ChartValues{"kubeletDirPath": "/custom/kubelet"},
					},
				},
			},
		}

		values := util.DeepMergeMaps(pcapps.RookCephPcAppsValues(configWith("/var/lib/k0s/kubelet")), pcApps)

		Expect(kubeletDirPath(values)).To(Equal("/custom/kubelet"))
	})
})
