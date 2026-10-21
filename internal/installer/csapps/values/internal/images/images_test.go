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

	It("registry that replaces BOM images -> every image moves onto the registry", func() {
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

	It("BOM image with a registry port -> port kept, tag split off", func() {
		imgs, err := images.New(testBOM(bomImages), nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(imgs.Ref("nginx")).To(
			Equal(images.Ref{Image: "localhost:5000/nginx", Tag: "1.31.5"}),
		)
	})

	It("BOM image without a tag -> release tag error", func() {
		untagged := map[string]string{"auth-service": imagePrefix + "auth-service@sha256:abc"}
		imgs, err := images.New(testBOM(untagged), nil)
		Expect(err).NotTo(HaveOccurred())

		_, err = imgs.ReleaseTag()

		Expect(err).To(MatchError(ContainSubstring("has no tag")))
	})
})
