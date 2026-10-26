// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package platform_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/platform"
)

const (
	namePrefix = "ghcr.io/codesphere-cloud/"
	digest     = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func fullImages() map[string]string {
	return map[string]string{
		"auth-service":          namePrefix + "auth-service:v1.0.0",
		"deployment-service":    namePrefix + "deployment-service:v1.0.1",
		"error-page-server":     namePrefix + "error-page-server:v1.0.2",
		"ide-frontend":          namePrefix + "ide-frontend:v1.0.3",
		"ide-service":           namePrefix + "ide-service:v1.0.4",
		"marketplace":           namePrefix + "marketplace:v1.0.5",
		"payment-service":       namePrefix + "payment-service:v1.0.6",
		"public-api-service":    namePrefix + "public-api-service:v1.0.7",
		"team-service":          namePrefix + "team-service:v1.0.8",
		"workspace-proxy":       namePrefix + "workspace-proxy:v1.0.9",
		"workspace-service":     namePrefix + "workspace-service:v1.0.10",
		"nginx":                 namePrefix + "nginx:1.31.5-alpine-otel",
		"opensearch":            namePrefix + "opensearch:3.8.0",
		"opensearchInit":        namePrefix + "busybox:1.37.0",
		"kubectl":               namePrefix + "kubectl:1.34.0",
		"observabilityOtel":     namePrefix + "otel-collector:0.120.0",
		"observabilityOtelInit": namePrefix + "curl:8.12.1",
	}
}

func testImages(bomImages map[string]string) images.Images {
	imgs, err := images.New(&bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: bomImages},
	}})
	Expect(err).NotTo(HaveOccurred())

	return imgs
}

func withImage(name, reference string) map[string]string {
	bomImages := fullImages()
	if reference == "" {
		delete(bomImages, name)
	} else {
		bomImages[name] = reference
	}

	return bomImages
}

var _ = Describe("Values", func() {
	It("all images in the BOM -> services without tag, router and OpenSearch split, others unchanged", func() {
		values, err := platform.Values(testImages(fullImages()))
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(platform.Platform{
			Services: platform.Services{
				PriorityClass: "prod-codesphere-deployment",
				Images: map[string]platform.ServiceImage{
					"auth_service":       {Image: namePrefix + "auth-service"},
					"deployment_service": {Image: namePrefix + "deployment-service"},
					"error_page_server":  {Image: namePrefix + "error-page-server"},
					"ide_frontend":       {Image: namePrefix + "ide-frontend"},
					"ide_service":        {Image: namePrefix + "ide-service"},
					"marketplace":        {Image: namePrefix + "marketplace"},
					"payment_service":    {Image: namePrefix + "payment-service"},
					"public_api_service": {Image: namePrefix + "public-api-service"},
					"team_service":       {Image: namePrefix + "team-service"},
					"workspace_proxy":    {Image: namePrefix + "workspace-proxy"},
					"workspace_service":  {Image: namePrefix + "workspace-service"},
				},
			},
			WorkspaceRouter: platform.WorkspaceRouter{
				Image:      platform.Image{Name: namePrefix + "nginx", Tag: "1.31.5-alpine-otel"},
				HAReplicas: 2,
			},
			WorkspaceObservability: platform.WorkspaceObservability{
				OpenSearchImage:        namePrefix + "opensearch",
				OpenSearchVersion:      "3.8.0",
				OpenSearchInitImage:    namePrefix + "busybox:1.37.0",
				MigrationKubectlImage:  namePrefix + "kubectl:1.34.0",
				OtelCollectorImage:     namePrefix + "otel-collector:0.120.0",
				OtelCollectorInitImage: namePrefix + "curl:8.12.1",
			},
		}))
	})

	It("service image missing from the BOM -> error", func() {
		_, err := platform.Values(testImages(withImage("team-service", "")))
		Expect(err).To(MatchError(ContainSubstring("image team-service not found in BOM")))
	})

	It("router image missing from the BOM -> error", func() {
		_, err := platform.Values(testImages(withImage("nginx", "")))
		Expect(err).To(MatchError(ContainSubstring("image nginx not found in BOM")))
	})

	It("OpenSearch image missing from the BOM -> error", func() {
		_, err := platform.Values(testImages(withImage("opensearch", "")))
		Expect(err).To(MatchError(ContainSubstring("image opensearch not found in BOM")))
	})

	It("auxiliary image missing from the BOM -> error", func() {
		_, err := platform.Values(testImages(withImage("observabilityOtel", "")))
		Expect(err).To(MatchError(ContainSubstring("image observabilityOtel not found in BOM")))
	})

	It("auxiliary image with neither tag nor digest -> error", func() {
		_, err := platform.Values(testImages(withImage("kubectl", namePrefix+"kubectl")))
		Expect(err).To(MatchError(ContainSubstring("missing a tag or digest")))
	})

	It("router image has only a digest -> error", func() {
		_, err := platform.Values(testImages(withImage("nginx", namePrefix+"nginx@"+digest)))
		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})

	It("OpenSearch image has only a digest -> error", func() {
		_, err := platform.Values(testImages(withImage("opensearch", namePrefix+"opensearch@"+digest)))
		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})

	It("images missing across services, router and observability -> every one in the error", func() {
		bomImages := fullImages()
		for _, name := range []string{"auth-service", "team-service", "nginx", "opensearch", "kubectl"} {
			delete(bomImages, name)
		}

		_, err := platform.Values(testImages(bomImages))
		Expect(err).To(SatisfyAll(
			MatchError(ContainSubstring("image auth-service not found in BOM")),
			MatchError(ContainSubstring("image team-service not found in BOM")),
			MatchError(ContainSubstring("image nginx not found in BOM")),
			MatchError(ContainSubstring("image opensearch not found in BOM")),
			MatchError(ContainSubstring("image kubectl not found in BOM")),
		))
	})
})
