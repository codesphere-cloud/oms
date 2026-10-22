// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package images_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
)

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testBOM(images map[string]string) *bom.Config {
	return &bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: images},
	}}
}

var _ = Describe("Image", func() {
	It("image in the BOM -> its reference unchanged", func() {
		imgs, err := images.New(testBOM(map[string]string{"nginx": "localhost:5000/nginx:1.31.5"}))
		Expect(err).NotTo(HaveOccurred())

		Expect(imgs.Image("nginx")).To(Equal("localhost:5000/nginx:1.31.5"))
	})

	It("image missing from the BOM -> error", func() {
		imgs, err := images.New(testBOM(map[string]string{}))
		Expect(err).NotTo(HaveOccurred())

		_, err = imgs.Image("nginx")
		Expect(err).To(MatchError(ContainSubstring("image nginx not found in BOM")))
	})
})

var _ = Describe("Ref", func() {
	DescribeTable("reference -> image, tag and digest",
		func(reference string, want images.Ref) {
			imgs, err := images.New(testBOM(map[string]string{"api": reference}))
			Expect(err).NotTo(HaveOccurred())

			Expect(imgs.Ref("api")).To(Equal(want))
		},
		Entry("tag", "ghcr.io/codesphere-cloud/api:1.2.3",
			images.Ref{Image: "ghcr.io/codesphere-cloud/api", Tag: "1.2.3"}),
		Entry("digest", "ghcr.io/codesphere-cloud/api@"+digest,
			images.Ref{Image: "ghcr.io/codesphere-cloud/api", Digest: digest}),
		Entry("tag and digest", "ghcr.io/codesphere-cloud/api:main@"+digest,
			images.Ref{Image: "ghcr.io/codesphere-cloud/api", Tag: "main", Digest: digest}),
		Entry("registry port and tag", "registry.example.com:5000/api:1.2.3",
			images.Ref{Image: "registry.example.com:5000/api", Tag: "1.2.3"}),
	)

	It("reference with neither tag nor digest -> error", func() {
		imgs, err := images.New(testBOM(map[string]string{"api": "registry.example.com:5000/api"}))
		Expect(err).NotTo(HaveOccurred())

		_, err = imgs.Ref("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag or digest")))
	})
})
