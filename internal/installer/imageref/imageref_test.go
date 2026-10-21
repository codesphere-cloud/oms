// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package imageref_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/imageref"
)

const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var _ = Describe("Split", func() {
	DescribeTable("reference -> repository, tag and digest",
		func(reference, repository, tag, digest string) {
			gotRepository, gotTag, gotDigest := imageref.Split(reference)

			Expect(gotRepository).To(Equal(repository))
			Expect(gotTag).To(Equal(tag))
			Expect(gotDigest).To(Equal(digest))
		},
		Entry("tag", "ghcr.io/codesphere-cloud/api:1.2.3", "ghcr.io/codesphere-cloud/api", "1.2.3", ""),
		Entry("digest", "ghcr.io/codesphere-cloud/api@"+digest, "ghcr.io/codesphere-cloud/api", "", digest),
		Entry("tag and digest", "ghcr.io/codesphere-cloud/api:main@"+digest, "ghcr.io/codesphere-cloud/api", "main", digest),
		Entry("registry port and tag", "registry.example.com:5000/api:1.2.3", "registry.example.com:5000/api", "1.2.3", ""),
		Entry("registry port without tag", "registry.example.com:5000/api", "registry.example.com:5000/api", "", ""),
		Entry("neither tag nor digest", "alpine", "alpine", "", ""),
	)
})

var _ = Describe("MirrorRepository", func() {
	DescribeTable("repository -> path below the mirror",
		func(repository, mirrored string) {
			Expect(imageref.MirrorRepository(repository)).To(Equal(mirrored))
		},
		Entry("registry with dots", "ghcr.io/codesphere-cloud/api", "ghcr_io/codesphere-cloud/api"),
		Entry("registry with a port", "registry.example.com:5000/api", "registry_example_com_5000/api"),
		Entry("no registry", "alpine/kubectl", "alpine/kubectl"),
		Entry("single segment", "alpine", "alpine"),
	)
})
