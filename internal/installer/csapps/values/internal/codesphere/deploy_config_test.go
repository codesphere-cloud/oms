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

var agent = files.ImageRef{BomRef: "workspace-agent-24.04"}

func deployConfigWith(flavors map[string]files.FlavorConfig) files.CodesphereConfig {
	return files.CodesphereConfig{
		DeployConfig: files.DeployConfig{Images: map[string]files.ImageConfig{
			"ubuntu-22.04": {
				Name:           "Ubuntu 22.04",
				SupportedUntil: "2027-04-01",
				Default:        ptr.To(true),
				Flavors:        flavors,
			},
		}},
	}
}

var _ = Describe("Deploy config", func() {
	It("minimal config -> default Ubuntu image from the BOM, pooled per plan", func() {
		conf := files.CodesphereConfig{
			Plans: files.PlansConfig{HostingPlans: map[int]files.HostingPlan{
				8: {PooledInstances: ptr.To(2)},
			}},
		}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

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
		conf := files.CodesphereConfig{
			Plans: files.PlansConfig{HostingPlans: map[int]files.HostingPlan{
				8: {},
				9: {GPUParts: 1, PooledInstances: ptr.To(1)},
			}},
		}
		imgs := testImagesWith(map[string]string{
			"workspace-agent-24.04.gpu": imagePrefix + "workspace-agent-24.04-gpu:v1.2.3",
		})

		values, err := codesphere.Values(conf, codesphere.Inputs{Images: imgs, Gateways: defaultTestGateways()})
		Expect(err).NotTo(HaveOccurred())

		flavors := deployImage(values, "ubuntu-24.04").Flavors
		Expect(flavors["default"].Pool).To(Equal(map[string]int{"8": 0}))
		Expect(flavors["gpu"]).To(Equal(codesphere.Flavor{
			Image: imagePrefix + "workspace-agent-24.04-gpu",
			Pool:  map[string]int{"9": 1},
		}))
	})

	It("configured deploy config -> its images and flavors", func() {
		conf := deployConfigWith(map[string]files.FlavorConfig{
			"default": {Image: agent, Pool: map[int]int{8: 1}},
			"gpu":     {Image: agent, Pool: map[int]int{9: 2}},
		})

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(deployImage(values, "ubuntu-22.04")).To(Equal(codesphere.DeployImage{
			Name:           "Ubuntu 22.04",
			SupportedUntil: "2027-04-01T00:00:00.000Z",
			Default:        ptr.To(true),
			Flavors: map[string]codesphere.Flavor{
				"default": {Image: imagePrefix + "workspace-agent-24.04", Pool: map[string]int{"8": 1}},
				"gpu":     {Image: imagePrefix + "workspace-agent-24.04", Pool: map[string]int{"9": 2}},
			},
		}))
	})

	It("flavor other than default and gpu -> error", func() {
		conf := deployConfigWith(map[string]files.FlavorConfig{
			"default": {Image: agent},
			"minimal": {Image: agent},
		})

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(ContainSubstring("unsupported flavor minimal")))
	})

	It("image without a default flavor -> error", func() {
		conf := deployConfigWith(map[string]files.FlavorConfig{"gpu": {Image: agent}})

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(ContainSubstring("the default flavor is required")))
	})

	It("invalid supportedUntil -> error", func() {
		conf := deployConfigWith(map[string]files.FlavorConfig{"default": {Image: agent}})
		image := conf.DeployConfig.Images["ubuntu-22.04"]
		image.SupportedUntil = "next year"
		conf.DeployConfig.Images["ubuntu-22.04"] = image

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(ContainSubstring(`invalid date "next year"`)))
	})

	It("flavor image missing from the BOM -> error", func() {
		conf := deployConfigWith(map[string]files.FlavorConfig{
			"default": {Image: files.ImageRef{BomRef: "workspace-agent-20.04"}},
		})

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(ContainSubstring("workspace-agent-20.04 not found")))
	})

	It("flavor with a Dockerfile and no registry -> error", func() {
		conf := files.CodesphereConfig{WorkspaceImages: &files.WorkspaceImagesConfig{
			Agent: &files.ImageRef{BomRef: "workspace-agent-24.04", Dockerfile: "Dockerfile.agent"},
		}}

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(ContainSubstring("Dockerfile.agent needs a registry server")))
	})

	It("flavor with a Dockerfile and an explicit image name -> that image, no registry needed", func() {
		conf := files.CodesphereConfig{WorkspaceImages: &files.WorkspaceImagesConfig{
			Agent: &files.ImageRef{ImageName: "registry.example.com/agent", Dockerfile: "Dockerfile.agent"},
		}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(deployImage(values, "ubuntu-24.04").Flavors["default"].Image).To(Equal("registry.example.com/agent"))
	})

	It("flavor with a Dockerfile and a registry -> image built from it", func() {
		conf := files.CodesphereConfig{WorkspaceImages: &files.WorkspaceImagesConfig{
			Agent: &files.ImageRef{BomRef: "workspace-agent-24.04", Dockerfile: "Dockerfile.agent"},
		}}
		registry := &files.RegistryConfig{Server: "registry.example.com"}

		values, err := codesphere.Values(conf, codesphere.Inputs{
			Images: defaultTestImages(), Gateways: defaultTestGateways(), Registry: registry,
		})
		Expect(err).NotTo(HaveOccurred())

		defaultFlavor := deployImage(values, "ubuntu-24.04").Flavors["default"]
		Expect(defaultFlavor.Image).To(Equal("registry.example.com/ubuntu-24.04-default"))
	})

	It("registry server with a trailing slash -> image without a double slash", func() {
		conf := files.CodesphereConfig{WorkspaceImages: &files.WorkspaceImagesConfig{
			Agent: &files.ImageRef{BomRef: "workspace-agent-24.04", Dockerfile: "Dockerfile.agent"},
		}}
		registry := &files.RegistryConfig{Server: "registry.example.com/private-cloud/"}

		values, err := codesphere.Values(conf, codesphere.Inputs{
			Images: defaultTestImages(), Gateways: defaultTestGateways(), Registry: registry,
		})
		Expect(err).NotTo(HaveOccurred())

		defaultFlavor := deployImage(values, "ubuntu-24.04").Flavors["default"]
		Expect(defaultFlavor.Image).To(Equal("registry.example.com/private-cloud/ubuntu-24.04-default"))
	})
})
