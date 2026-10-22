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

func testImages(reference string) images.Images {
	imgs, err := images.New(testBOM(map[string]string{"api": reference}))
	Expect(err).NotTo(HaveOccurred())

	return imgs
}

var _ = Describe("Named", func() {
	DescribeTable("reference -> its name without tag and digest",
		func(reference string, want string) {
			named, err := testImages(reference).Named("api")
			Expect(err).NotTo(HaveOccurred())

			Expect(named.Name()).To(Equal(want))
		},
		Entry("tag", "ghcr.io/codesphere-cloud/api:1.2.3", "ghcr.io/codesphere-cloud/api"),
		Entry("digest", "ghcr.io/codesphere-cloud/api@"+digest, "ghcr.io/codesphere-cloud/api"),
		Entry("tag and digest", "ghcr.io/codesphere-cloud/api:main@"+digest, "ghcr.io/codesphere-cloud/api"),
		Entry("registry port and tag", "registry.example.com:5000/api:1.2.3", "registry.example.com:5000/api"),
		Entry("no registry", "nginx:1.31.5", "nginx"),
	)

	It("reference with neither tag nor digest -> error", func() {
		_, err := testImages("registry.example.com:5000/api").Named("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag or digest")))
	})

	It("malformed reference -> error", func() {
		_, err := testImages("ghcr.io/codesphere-cloud/API:1.2.3").Named("api")
		Expect(err).To(MatchError(ContainSubstring("failed to parse image")))
	})
})

var _ = Describe("Tagged", func() {
	DescribeTable("tagged reference -> its tag",
		func(reference string, want string) {
			tagged, err := testImages(reference).Tagged("api")
			Expect(err).NotTo(HaveOccurred())

			Expect(tagged.Tag()).To(Equal(want))
		},
		Entry("tag", "ghcr.io/codesphere-cloud/api:1.2.3", "1.2.3"),
		Entry("tag and digest", "ghcr.io/codesphere-cloud/api:main@"+digest, "main"),
	)

	It("digest-only reference -> error", func() {
		_, err := testImages("ghcr.io/codesphere-cloud/api@" + digest).Tagged("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})
})
