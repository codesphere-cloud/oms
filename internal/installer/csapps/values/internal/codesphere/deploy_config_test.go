// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Deploy config", func() {
	It("minimal config -> default Ubuntu image from the BOM, pooled per plan", func() {
		values := input{conf: files.CodesphereConfig{
			Plans: files.PlansConfig{HostingPlans: map[int]files.HostingPlan{
				8: {PooledInstances: ptr.To(2)},
			}},
		}}.mustValues()

		Expect(values.DeployConfig).To(Equal(codesphere.DeployConfig{
			Images: map[string]codesphere.DeployImage{
				"ubuntu-24.04": {
					Name:           "Ubuntu 24.04",
					SupportedUntil: "2028-05-31T00:00:00.000Z",
					Flavors: map[string]codesphere.Flavor{
						"default": {
							Image: imagePrefix + "workspace-agent-24.04",
							Pool:  map[string]int{"8": 2},
						},
					},
				},
			},
		}))
	})

	It("BOM with a GPU agent -> gpu flavor pools only the GPU plans", func() {
		values := input{
			conf: files.CodesphereConfig{
				Plans: files.PlansConfig{HostingPlans: map[int]files.HostingPlan{
					8: {},
					9: {GPUParts: 1, PooledInstances: ptr.To(1)},
				}},
			},
			images: testImages(map[string]string{
				"workspace-agent-24.04.gpu": imagePrefix + "workspace-agent-24.04-gpu:v1.2.3",
			}),
		}.mustValues()

		flavors := deployImage(values, "ubuntu-24.04").Flavors
		Expect(flavors["default"].Pool).To(Equal(map[string]int{"8": 0}))
		Expect(flavors["gpu"]).To(Equal(codesphere.Flavor{
			Image: imagePrefix + "workspace-agent-24.04-gpu",
			Pool:  map[string]int{"9": 1},
		}))
	})

	It("configured deploy config -> only the default and gpu flavors", func() {
		agent := files.ImageRef{BomRef: "workspace-agent-24.04"}
		values := input{conf: files.CodesphereConfig{
			DeployConfig: files.DeployConfig{Images: map[string]files.ImageConfig{
				"ubuntu-22.04": {
					Name:           "Ubuntu 22.04",
					SupportedUntil: "2027-04-01",
					Default:        ptr.To(true),
					Flavors: map[string]files.FlavorConfig{
						"default": {Image: agent, Pool: map[int]int{8: 1}},
						"minimal": {Image: agent, Pool: map[int]int{8: 1}},
					},
				},
			}},
		}}.mustValues()

		image := deployImage(values, "ubuntu-22.04")
		Expect(image.Default).To(HaveValue(BeTrue()))
		Expect(image.SupportedUntil).To(Equal("2027-04-01T00:00:00.000Z"))
		Expect(image.Flavors).To(HaveLen(1))
		Expect(image.Flavors).To(HaveKey("default"))
	})

	It("flavor with a Dockerfile and a registry -> image built from it", func() {
		values := input{
			conf: files.CodesphereConfig{WorkspaceImages: &files.WorkspaceImagesConfig{
				Agent: &files.ImageRef{BomRef: "workspace-agent-24.04", Dockerfile: "Dockerfile.agent"},
			}},
			registry: &files.RegistryConfig{Server: "registry.example.com"},
		}.mustValues()

		defaultFlavor := deployImage(values, "ubuntu-24.04").Flavors["default"]
		Expect(defaultFlavor.Image).To(Equal("registry.example.com/ubuntu-24.04-default"))
	})
})
