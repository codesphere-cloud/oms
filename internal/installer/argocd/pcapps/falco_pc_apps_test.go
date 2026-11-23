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

var _ = Describe("FalcoPcAppsValues", func() {
	// The registry block of the falco-deployment application in the rendered values.
	registryValues := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		falco, ok := apps["falco-deployment"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a falco-deployment application")

		valuesObject, ok := falco["valuesObject"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a valuesObject")

		registry, ok := valuesObject["registry"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a registry block")

		return registry
	}

	It("returns nil when no registry is configured", func() {
		values, err := pcapps.FalcoPcAppsValues(&files.RootConfig{})

		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(BeNil())
	})

	It("returns nil when the registry server is empty", func() {
		values, err := pcapps.FalcoPcAppsValues(&files.RootConfig{Registry: &files.RegistryConfig{}})

		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(BeNil())
	})

	It("sets the registry host as the plugin registry name", func() {
		values, err := pcapps.FalcoPcAppsValues(&files.RootConfig{
			Registry: &files.RegistryConfig{Server: "mirror.example.com"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(registryValues(values)).To(Equal(files.ChartValues{"name": "mirror.example.com"}))
	})

	It("keeps the port and drops the repository path prefix", func() {
		values, err := pcapps.FalcoPcAppsValues(&files.RootConfig{
			Registry: &files.RegistryConfig{Server: "mirror.example.com:5000/team/"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(registryValues(values)).To(Equal(files.ChartValues{"name": "mirror.example.com:5000"}))
	})

	It("rejects a registry server with a scheme", func() {
		_, err := pcapps.FalcoPcAppsValues(&files.RootConfig{
			Registry: &files.RegistryConfig{Server: "https://mirror.example.com"},
		})

		Expect(err).To(MatchError(ContainSubstring("invalid registry server for falco-deployment")))
	})

	It("lets the pcApps registry name win over the configured registry", func() {
		inputConfig := &files.RootConfig{
			Registry: &files.RegistryConfig{Server: "mirror.example.com"},
			PcApps: files.ChartValues{
				"applications": files.ChartValues{
					"falco-deployment": files.ChartValues{
						"valuesObject": files.ChartValues{
							"registry": files.ChartValues{"name": "plugins.example.com"},
						},
					},
				},
			},
		}

		values, err := pcapps.FalcoPcAppsValues(inputConfig)
		Expect(err).NotTo(HaveOccurred())

		merged := util.DeepMergeMaps(values, inputConfig.PcApps)

		Expect(registryValues(merged)).To(HaveKeyWithValue("name", "plugins.example.com"))
	})
})
