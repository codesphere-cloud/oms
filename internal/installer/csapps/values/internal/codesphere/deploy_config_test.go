// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Deploy config", func() {
	It("minimal config -> default Ubuntu image from the BOM, pooled per plan", func() {
		values := input{conf: `
			plans:
			  hostingPlans:
			    8:
			      cpuTenth: 10
			      gpuParts: 0
			      memoryMb: 2048
			      storageMb: 20480
			      tempStorageMb: 1500
			      pooledInstances: 2
		`}.mustValues()

		Expect(values["deployConfig"]).To(Equal(map[string]any{
			"images": map[string]any{
				"ubuntu-24.04": map[string]any{
					"name":           "Ubuntu 24.04",
					"supportedUntil": "2028-05-31T00:00:00.000Z",
					"flavors": map[string]any{
						"default": map[string]any{
							"image": imagePrefix + "workspace-agent-24.04",
							"pool":  map[string]any{"8": 2},
						},
					},
				},
			},
		}))
		Expect(values["poolDefinition"]).To(Equal(map[string]any{"8": 2}))
	})

	It("BOM with a GPU agent -> gpu flavor pools only the GPU plans", func() {
		values := input{
			conf: `
				plans:
				  hostingPlans:
				    8:
				      cpuTenth: 10
				      gpuParts: 0
				      memoryMb: 2048
				      storageMb: 20480
				      tempStorageMb: 1500
				    9:
				      cpuTenth: 10
				      gpuParts: 1
				      memoryMb: 2048
				      storageMb: 20480
				      tempStorageMb: 1500
				      pooledInstances: 1
			`,
			images: testImages(map[string]string{
				"workspace-agent-24.04.gpu": imagePrefix + "workspace-agent-24.04-gpu:v1.2.3",
			}),
		}.mustValues()

		flavors := deployImage(values, "ubuntu-24.04")["flavors"].(map[string]any)
		Expect(flavors["default"]).To(HaveKeyWithValue("pool", map[string]any{"8": 0}))
		Expect(flavors["gpu"]).To(Equal(map[string]any{
			"image": imagePrefix + "workspace-agent-24.04-gpu",
			"pool":  map[string]any{"9": 1},
		}))
	})

	It("configured deploy config -> only the default and gpu flavors", func() {
		values := input{conf: `
			deployConfig:
			  images:
			    ubuntu-22.04:
			      name: Ubuntu 22.04
			      supportedUntil: "2027-04-01"
			      default: true
			      flavors:
			        default: {image: {bomRef: workspace-agent-24.04}, pool: {8: 1}}
			        minimal: {image: {bomRef: workspace-agent-24.04}, pool: {8: 1}}
		`}.mustValues()

		image := deployImage(values, "ubuntu-22.04")
		Expect(image["default"]).To(BeTrue())
		Expect(image["supportedUntil"]).To(Equal("2027-04-01T00:00:00.000Z"))
		Expect(image["flavors"]).To(HaveLen(1))
		Expect(image["flavors"]).To(HaveKey("default"))
	})

	It("flavor with a Dockerfile and a registry -> image built from it", func() {
		values := input{
			conf: `
				workspaceImages:
				  agent: {bomRef: workspace-agent-24.04, dockerfile: Dockerfile.agent}
			`,
			registry: &files.RegistryConfig{Server: "registry.example.com"},
		}.mustValues()

		defaultFlavor := deployImage(values, "ubuntu-24.04")["flavors"].(map[string]any)["default"]
		Expect(defaultFlavor).To(
			HaveKeyWithValue("image", "registry.example.com/ubuntu-24.04-default"),
		)
	})
})
