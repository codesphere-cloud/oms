// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("GetBuildForDownload", func() {
	It("Extracts a build with a single matching artifact", func() {

		build := portal.Build{
			Artifacts: []portal.Artifact{
				{Filename: "a.txt"},
				{Filename: "b.txt"},
				{Filename: "c.txt"},
			},
		}

		expectedBuild := portal.Build{
			Artifacts: []portal.Artifact{
				{Filename: "b.txt"},
			},
		}

		res, err := build.GetBuildForDownload("b.txt")
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(expectedBuild))
	})

})

var _ = Describe("BuildPackageFilename", func() {
	Describe("BuildPackageFilename method", func() {
		It("generates filename with long hash", func() {
			build := portal.Build{
				Version: "v1.2.3",
				Hash:    "abc1234567890def",
			}
			filename := build.BuildPackageFilename("installer.tar.gz")
			Expect(filename).To(Equal("v1.2.3-abc1234567890def-installer.tar.gz"))
		})

		It("handles short hash", func() {
			build := portal.Build{
				Version: "v1.2.3",
				Hash:    "abc123",
			}
			filename := build.BuildPackageFilename("installer.tar.gz")
			Expect(filename).To(Equal("v1.2.3-abc123-installer.tar.gz"))
		})

		It("replaces slashes in version with dashes", func() {
			build := portal.Build{
				Version: "feature/my-branch",
				Hash:    "abc1234567890",
			}
			filename := build.BuildPackageFilename("installer-lite.tar.gz")
			Expect(filename).To(Equal("feature-my-branch-abc1234567890-installer-lite.tar.gz"))
		})
	})

	Describe("BuildPackageFilenameFromParts function", func() {
		It("generates filename from parts with long hash", func() {
			filename := portal.BuildPackageFilenameFromParts("v1.2.3", "abc1234567890def", "installer.tar.gz")
			Expect(filename).To(Equal("v1.2.3-abc1234567890def-installer.tar.gz"))
		})

		It("handles branch versions with slashes", func() {
			filename := portal.BuildPackageFilenameFromParts("feature/test", "abc1234567890", "installer.tar.gz")
			Expect(filename).To(Equal("feature-test-abc1234567890-installer.tar.gz"))
		})

		It("handles exact 10 character hash", func() {
			filename := portal.BuildPackageFilenameFromParts("v1.0.0", "1234567890", "installer.tar.gz")
			Expect(filename).To(Equal("v1.0.0-1234567890-installer.tar.gz"))
		})

		It("handles empty hash", func() {
			filename := portal.BuildPackageFilenameFromParts("v1.0.0", "", "installer.tar.gz")
			Expect(filename).To(Equal("v1.0.0--installer.tar.gz"))
		})

		It("handles empty version", func() {
			filename := portal.BuildPackageFilenameFromParts("", "abc1234567", "installer.tar.gz")
			Expect(filename).To(Equal("-abc1234567-installer.tar.gz"))
		})

		It("handles multiple slashes in version", func() {
			filename := portal.BuildPackageFilenameFromParts("feature/sub/branch/v1", "abc1234567", "installer.tar.gz")
			Expect(filename).To(Equal("feature-sub-branch-v1-abc1234567-installer.tar.gz"))
		})
	})
})
