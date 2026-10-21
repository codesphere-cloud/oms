// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package images_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const imagePrefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

func testBOM(images map[string]string) *bom.Config {
	return &bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: images},
	}}
}

var _ = Describe("New", func() {
	bomImages := map[string]string{
		"auth-service": imagePrefix + "auth-service:v1.2.3",
		"nginx":        "localhost:5000/nginx:1.31.5",
	}

	It("registry that replaces BOM images -> every image reference points at the registry", func() {
		registry := &files.RegistryConfig{
			Server:             "registry.example.com:5000",
			ReplaceImagesInBom: true,
		}
		imgs, err := images.New(testBOM(bomImages), registry)
		Expect(err).NotTo(HaveOccurred())

		Expect(imgs.Image("auth-service")).To(Equal(
			"registry.example.com:5000/ghcr_io/codesphere-cloud/codesphere-monorepo/auth-service:v1.2.3",
		))
		Expect(imgs.Image("nginx")).To(
			Equal("registry.example.com:5000/localhost_5000/nginx:1.31.5"),
		)
	})

	It("registry that keeps BOM images -> images unchanged", func() {
		registry := &files.RegistryConfig{Server: "registry.example.com"}
		imgs, err := images.New(testBOM(bomImages), registry)
		Expect(err).NotTo(HaveOccurred())

		Expect(imgs.Image("auth-service")).To(Equal(imagePrefix + "auth-service:v1.2.3"))
	})
})
