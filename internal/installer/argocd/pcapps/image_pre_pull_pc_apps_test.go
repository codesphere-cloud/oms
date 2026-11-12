// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("ImagePrePullPcAppsValues", func() {
	const prefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

	testBOM := func(extra map[string]string) *bom.Config {
		images := map[string]string{
			"auth-service":          prefix + "auth-service:v1.2.3",
			"deployment-service":    prefix + "deployment-service:v1.2.3",
			"error-page-server":     prefix + "error-page-server:v1.2.3",
			"ide-frontend":          prefix + "ide-frontend:v1.2.3",
			"ide-service":           prefix + "ide-service:v1.2.3",
			"marketplace":           prefix + "marketplace:v1.2.3",
			"payment-service":       prefix + "payment-service:v1.2.3",
			"public-api-service":    prefix + "public-api-service:v1.2.3",
			"team-service":          prefix + "team-service:v1.2.3",
			"workspace-proxy":       prefix + "workspace-proxy:v1.2.3",
			"workspace-service":     prefix + "workspace-service:v1.2.3",
			"workspace-agent-24.04": prefix + "workspace-agent-24.04:v1.2.3",
			"workspace-server":      prefix + "workspace-server:v1.2.3",
			"vpn":                   prefix + "vpn:v1.2.3",
			"nginx":                 "ghcr.io/codesphere-cloud/docker/nginx:1.31.5",
			"frontendGatewayNginx":  "ghcr.io/codesphere-cloud/docker/nginx-gateway:1.31.5",
			"opensearch":            "ghcr.io/codesphere-cloud/docker/opensearch:3.8.0",
			"opensearchInit":        "ghcr.io/codesphere-cloud/docker/busybox:1.37.0",
			"kubectl":               "ghcr.io/codesphere-cloud/docker/kubectl:1.34.0",
			"observabilityOtel":     "ghcr.io/codesphere-cloud/docker/otel:0.160.0",
			"observabilityOtelInit": "ghcr.io/codesphere-cloud/docker/curl:8.20.0",
		}
		for name, image := range extra {
			images[name] = image
		}

		return &bom.Config{Components: map[string]bom.ComponentConfig{
			"codesphere": {ContainerImages: images},
		}}
	}

	testConfig := func() *files.RootConfig {
		config := files.NewRootConfig()
		config.Datacenter = files.DatacenterConfig{ID: 1, Name: "main", City: "Karlsruhe", CountryCode: "DE"}
		config.Postgres = files.PostgresConfig{Mode: "external", ServerAddress: "postgres.example.com"}
		config.Kubernetes = files.KubernetesConfig{PodCIDR: "10.1.0.0/16", ServiceCIDR: "10.2.0.0/16"}
		config.Codesphere = files.CodesphereConfig{
			Domain:                     "codesphere.example.com",
			WorkspaceHostingBaseDomain: "ws.example.com",
			PublicIP:                   "203.0.113.10",
			CustomDomains:              files.CustomDomainsConfig{CNameBaseDomain: "custom.example.com"},
			Plans: files.PlansConfig{
				HostingPlans:   map[int]files.HostingPlan{8: {CPUTenth: 10, MemoryMb: 2048, StorageMb: 20480, TempStorageMb: 1500}},
				WorkspacePlans: map[int]files.WorkspacePlan{},
			},
		}

		return &config
	}

	// The valuesObject of the image-pre-pull application in the rendered values.
	appValues := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		app, ok := apps["image-pre-pull"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an image-pre-pull application")
		Expect(app["enabled"]).To(BeTrue())

		valuesObject, ok := app["valuesObject"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a valuesObject")

		return valuesObject
	}

	images := func(valuesObject files.ChartValues, list string) []map[string]string {
		all, ok := valuesObject["images"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an images map")

		result, ok := all[list].([]map[string]string)
		Expect(ok).To(BeTrue(), "expected an images list")

		return result
	}

	It("enables the app with the workspace and agent images of the release", func() {
		bomConfig := testBOM(map[string]string{
			"custom-workspace-image-init": prefix + "custom-workspace-image-init:v1.2.3",
			"workspace-router":            prefix + "workspace-router:v1.2.3",
		})

		values, err := pcapps.ImagePrePullPcAppsValues(testConfig(), bomConfig)

		Expect(err).NotTo(HaveOccurred())

		valuesObject := appValues(values)
		Expect(valuesObject["priorityClassName"]).To(Equal("prod-codesphere-deployment"))
		Expect(valuesObject["allowOnControlPlane"]).To(BeFalse())
		Expect(images(valuesObject, "workspace")).To(Equal([]map[string]string{
			{"name": "workspace-server", "image": prefix + "workspace-server:v1.2.3"},
			{"name": "vpn", "image": prefix + "vpn:v1.2.3"},
			{"name": "custom-workspace-image-init", "image": prefix + "custom-workspace-image-init:v1.2.3"},
			{"name": "workspace-router", "image": prefix + "workspace-router:v1.2.3"},
		}))
		Expect(images(valuesObject, "agents")).To(Equal([]map[string]string{
			{"name": "agent-ubuntu-24-04-default", "image": prefix + "workspace-agent-24.04:v1.2.3"},
		}))
	})

	It("leaves out the router and init image when the BOM has none", func() {
		values, err := pcapps.ImagePrePullPcAppsValues(testConfig(), testBOM(nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(images(appValues(values), "workspace")).To(Equal([]map[string]string{
			{"name": "workspace-server", "image": prefix + "workspace-server:v1.2.3"},
			{"name": "vpn", "image": prefix + "vpn:v1.2.3"},
		}))
	})

	It("uses the images and the control plane flag of the Codesphere override", func() {
		config := testConfig()
		config.Codesphere.Override = map[string]any{
			"global": map[string]any{
				"allowWorkspacesOnControlPlane": true,
				"workspaceImages":               map[string]any{"router": "registry.example.com/router"},
			},
		}

		values, err := pcapps.ImagePrePullPcAppsValues(config, testBOM(nil))

		Expect(err).NotTo(HaveOccurred())

		valuesObject := appValues(values)
		Expect(valuesObject["allowOnControlPlane"]).To(BeTrue())
		Expect(images(valuesObject, "workspace")).To(ContainElement(
			map[string]string{"name": "workspace-router", "image": "registry.example.com/router:v1.2.3"},
		))
	})

	It("returns nil when pre-pulling is turned off", func() {
		config := testConfig()
		config.Codesphere.PrePullImages = ptr.To(false)

		values, err := pcapps.ImagePrePullPcAppsValues(config, testBOM(nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(BeNil())
	})

	It("fails when the Codesphere values cannot be built", func() {
		_, err := pcapps.ImagePrePullPcAppsValues(testConfig(), &bom.Config{})

		Expect(err).To(HaveOccurred())
	})
})
