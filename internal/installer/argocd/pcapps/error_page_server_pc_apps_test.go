// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("ErrorPageServerPcAppsValues", func() {
	const prefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

	// installerBom has every image the Codesphere values need.
	installerBom := func() *bom.Config {
		images := map[string]string{
			"auth-service":          prefix + "auth-service:v1.2.3",
			"deployment-service":    prefix + "deployment-service:v1.2.3",
			"ide-frontend":          prefix + "ide-frontend:v1.2.3",
			"ide-service":           prefix + "ide-service:v1.2.3",
			"marketplace":           prefix + "marketplace:v1.2.3",
			"payment-service":       prefix + "payment-service:v1.2.3",
			"public-api-service":    prefix + "public-api-service:v1.2.3",
			"team-service":          prefix + "team-service:v1.2.3",
			"workspace-proxy":       prefix + "workspace-proxy:v1.2.3",
			"workspace-service":     prefix + "workspace-service:v1.2.3",
			"workspace-agent-24.04": prefix + "workspace-agent-24.04:v1.2.3",
			"workspace-agent":       prefix + "workspace-agent:v1.2.3",
			"workspace-server":      prefix + "workspace-server:v1.2.3",
			"vpn":                   prefix + "vpn:v1.2.3",
			"nginx":                 "ghcr.io/codesphere-cloud/docker/nginx:1.31.5",
			"frontendGatewayNginx":  "ghcr.io/codesphere-cloud/docker/nginx-gateway:1.31.5",
			"opensearch":            "ghcr.io/codesphere-cloud/docker/opensearch:3.8.0",
			"opensearchInit":        "ghcr.io/codesphere-cloud/docker/busybox:1.37.0",
			"kubectl":               "ghcr.io/codesphere-cloud/docker/kubectl:1.34.0",
			"observabilityOtel":     "ghcr.io/codesphere-cloud/docker/otel:0.160.0",
			"observabilityOtelInit": "ghcr.io/codesphere-cloud/docker/curl:8.20.0",
			"error-page-server":     prefix + "error-page-server:v1.2.3",
		}

		return &bom.Config{Components: map[string]bom.ComponentConfig{
			"codesphere": {ContainerImages: images},
		}}
	}

	config := func() *files.RootConfig {
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
				HostingPlans: map[int]files.HostingPlan{
					8: {CPUTenth: 10, MemoryMb: 2048, StorageMb: 20480, TempStorageMb: 1500},
				},
				WorkspacePlans: map[int]files.WorkspacePlan{},
			},
		}

		return &config
	}

	It("passes the Codesphere globals and leaves enabled to the pc-applications chart", func() {
		values, err := pcapps.ErrorPageServerPcAppsValues(config(), installerBom())
		Expect(err).NotTo(HaveOccurred())

		app := values["applications"].(files.ChartValues)["error-page-server"].(files.ChartValues)
		Expect(app).NotTo(HaveKey("enabled"))

		global := app["valuesObject"].(map[string]any)["global"].(map[string]any)
		Expect(global["hostName"]).To(Equal("codesphere.example.com"))
		Expect(global["env"]).To(Equal("prod"))
		Expect(global["externalSecretsVaultSecret"]).To(Equal("cs-vault"))
		Expect(global["externalSecretsSyncTrigger"]).NotTo(BeEmpty())
		Expect(global["services"]).To(HaveKeyWithValue("error_page_server",
			map[string]any{"image": prefix + "error-page-server"}))
	})

	It("BOM without a codesphere component -> error", func() {
		_, err := pcapps.ErrorPageServerPcAppsValues(config(), &bom.Config{})

		Expect(err).To(MatchError(ContainSubstring("failed to build the Codesphere values")))
	})
})
