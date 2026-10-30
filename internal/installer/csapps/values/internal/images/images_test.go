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

func testImages(reference string) images.Images {
	imgs, err := images.New(testBOM(map[string]string{"api": reference}))
	Expect(err).NotTo(HaveOccurred())

	return imgs
}

var _ = Describe("Has", func() {
	It("image in the BOM -> true", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:1.2.3").Has("api")).To(BeTrue())
	})

	It("image missing from the BOM -> false", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:1.2.3").Has("auth")).To(BeFalse())
	})
})

var _ = Describe("Reference", func() {
	It("image in the BOM -> its reference unchanged", func() {
		Expect(testImages("localhost:5000/api:1.31.5").Reference("api")).To(Equal("localhost:5000/api:1.31.5"))
	})

	It("image with tag and digest -> its reference unchanged", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:main@" + digest).Reference("api")).
			To(Equal("ghcr.io/codesphere-cloud/api:main@" + digest))
	})

	It("image missing from the BOM -> error", func() {
		imgs, err := images.New(testBOM(map[string]string{}))
		Expect(err).NotTo(HaveOccurred())

		_, err = imgs.Reference("api")
		Expect(err).To(MatchError(ContainSubstring("image api not found in BOM")))
	})

	It("reference with neither tag nor digest -> error", func() {
		_, err := testImages("registry.example.com:5000/api").Reference("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag or digest")))
	})

	It("malformed reference -> error", func() {
		_, err := testImages("ghcr.io/codesphere-cloud/API:1.2.3").Reference("api")
		Expect(err).To(MatchError(ContainSubstring("failed to parse image")))
	})
})

var _ = Describe("Name", func() {
	It("reference with tag -> registry and repository", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:1.2.3").Name("api")).To(Equal("ghcr.io/codesphere-cloud/api"))
	})

	It("reference with digest -> registry and repository", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api@" + digest).Name("api")).To(Equal("ghcr.io/codesphere-cloud/api"))
	})

	It("reference with tag and digest -> registry and repository", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:main@" + digest).Name("api")).To(Equal("ghcr.io/codesphere-cloud/api"))
	})

	It("reference with registry port and tag -> registry with port and repository", func() {
		Expect(testImages("registry.example.com:5000/api:1.2.3").Name("api")).To(Equal("registry.example.com:5000/api"))
	})

	It("reference without registry -> repository only", func() {
		Expect(testImages("nginx:1.31.5").Name("api")).To(Equal("nginx"))
	})

	It("reference with neither tag nor digest -> error", func() {
		_, err := testImages("registry.example.com:5000/api").Name("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag or digest")))
	})

	It("malformed reference -> error", func() {
		_, err := testImages("ghcr.io/codesphere-cloud/API:1.2.3").Name("api")
		Expect(err).To(MatchError(ContainSubstring("failed to parse image")))
	})
})

var _ = Describe("Tag", func() {
	It("reference with tag -> its tag", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:1.2.3").Tag("api")).To(Equal("1.2.3"))
	})

	It("reference with tag and digest -> its tag", func() {
		Expect(testImages("ghcr.io/codesphere-cloud/api:main@" + digest).Tag("api")).To(Equal("main"))
	})

	It("reference with registry port and tag -> its tag, not the port", func() {
		Expect(testImages("registry.example.com:5000/api:1.2.3").Tag("api")).To(Equal("1.2.3"))
	})

	It("digest-only reference -> error", func() {
		_, err := testImages("ghcr.io/codesphere-cloud/api@" + digest).Tag("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})
})

var _ = Describe("NameAndTag", func() {
	It("reference with registry port and tag -> name with port and tag", func() {
		name, tag, err := testImages("registry.example.com:5000/api:1.2.3").NameAndTag("api")
		Expect(err).NotTo(HaveOccurred())

		Expect(name).To(Equal("registry.example.com:5000/api"))
		Expect(tag).To(Equal("1.2.3"))
	})

	It("digest-only reference -> error", func() {
		_, _, err := testImages("ghcr.io/codesphere-cloud/api@" + digest).NameAndTag("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})

	It("image missing from the BOM -> error", func() {
		imgs, err := images.New(testBOM(map[string]string{}))
		Expect(err).NotTo(HaveOccurred())

		_, _, err = imgs.NameAndTag("api")
		Expect(err).To(MatchError(ContainSubstring("image api not found in BOM")))
	})
})

var _ = Describe("Image", func() {
	It("reference with registry port and tag -> name with port and tag", func() {
		Expect(testImages("registry.example.com:5000/api:1.2.3").Image("api")).To(Equal(images.Image{
			Name: "registry.example.com:5000/api",
			Tag:  "1.2.3",
		}))
	})

	It("digest-only reference -> error", func() {
		_, err := testImages("ghcr.io/codesphere-cloud/api@" + digest).Image("api")
		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})
})
