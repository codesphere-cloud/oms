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
	DescribeTable("reference -> name, tag and digest",
		func(reference, name, tag, digest string) {
			gotName, gotTag, gotDigest := imageref.Split(reference)

			Expect(gotName).To(Equal(name))
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

var _ = Describe("MirroredName", func() {
	DescribeTable("name and mirror registry -> name on the mirror",
		func(name, mirrorRegistry, mirrored string) {
			Expect(imageref.MirroredName(name, mirrorRegistry)).To(Equal(mirrored))
		},
		Entry("registry with dots", "ghcr.io/codesphere-cloud/api", "registry.example.com:5000",
			"registry.example.com:5000/ghcr_io/codesphere-cloud/api"),
		Entry("registry with a port", "localhost:5000/nginx", "registry.example.com:5000",
			"registry.example.com:5000/localhost_5000/nginx"),
		Entry("mirror with a path", "ghcr.io/codesphere-cloud/api", "registry.example.com/mirror/",
			"registry.example.com/mirror/ghcr_io/codesphere-cloud/api"),
		Entry("no registry", "alpine/kubectl", "10.10.0.2:5000", "10.10.0.2:5000/alpine/kubectl"),
		Entry("single segment", "alpine", "10.10.0.2:5000", "10.10.0.2:5000/alpine"),
	)
})
