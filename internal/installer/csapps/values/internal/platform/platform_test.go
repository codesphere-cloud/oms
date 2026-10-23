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

const namePrefix = "ghcr.io/codesphere-cloud/"

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

		Expect(values).To(Equal(map[string]any{
			"services": map[string]any{
				"priorityClass":      "prod-codesphere-deployment",
				"auth_service":       map[string]any{"image": namePrefix + "auth-service"},
				"deployment_service": map[string]any{"image": namePrefix + "deployment-service"},
				"error_page_server":  map[string]any{"image": namePrefix + "error-page-server"},
				"ide_frontend":       map[string]any{"image": namePrefix + "ide-frontend"},
				"ide_service":        map[string]any{"image": namePrefix + "ide-service"},
				"marketplace":        map[string]any{"image": namePrefix + "marketplace"},
				"payment_service":    map[string]any{"image": namePrefix + "payment-service"},
				"public_api_service": map[string]any{"image": namePrefix + "public-api-service"},
				"team_service":       map[string]any{"image": namePrefix + "team-service"},
				"workspace_proxy":    map[string]any{"image": namePrefix + "workspace-proxy"},
				"workspace_service":  map[string]any{"image": namePrefix + "workspace-service"},
			},
			"workspaceRouter": map[string]any{
				"image":      map[string]any{"name": namePrefix + "nginx", "tag": "1.31.5-alpine-otel"},
				"haReplicas": 2,
			},
			"workspaceObservability": map[string]any{
				"openSearchImage":        namePrefix + "opensearch",
				"openSearchVersion":      "3.8.0",
				"openSearchInitImage":    namePrefix + "busybox:1.37.0",
				"migrationKubectlImage":  namePrefix + "kubectl:1.34.0",
				"otelCollectorImage":     namePrefix + "otel-collector:0.120.0",
				"otelCollectorInitImage": namePrefix + "curl:8.12.1",
			},
		}))
	})

	DescribeTable("required image missing from the BOM -> error",
		func(name string) {
			_, err := platform.Values(testImages(withImage(name, "")))
			Expect(err).To(MatchError(ContainSubstring("image " + name + " not found in BOM")))
		},
		Entry("service", "team-service"),
		Entry("router", "nginx"),
		Entry("OpenSearch", "opensearch"),
		Entry("auxiliary", "observabilityOtel"),
	)

	DescribeTable("image that needs a tag has only a digest -> error",
		func(name string) {
			digestOnly := namePrefix + name + "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
			_, err := platform.Values(testImages(withImage(name, digestOnly)))
			Expect(err).To(MatchError(ContainSubstring("missing a tag")))
		},
		Entry("router", "nginx"),
		Entry("OpenSearch", "opensearch"),
	)
})
