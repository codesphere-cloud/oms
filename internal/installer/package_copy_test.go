// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"errors"
	"os"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Package artifact copying", func() {
	Describe("PackageImageDestination", func() {
		It("preserves the source repository path and tag below the destination", func() {
			destination, err := installer.PackageImageDestination(
				"ghcr.io/codesphere-cloud/api:1.2.3",
				"registry.example.com/private-cloud/",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("registry.example.com/private-cloud/codesphere-cloud/api:1.2.3"))
		})

		It("keeps the source registry of other images in its own path segment", func() {
			destination, err := installer.PackageImageDestination(
				"quay.io/ceph/ceph:v18.2",
				"10.10.0.2:5000",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("10.10.0.2:5000/quay_io/ceph/ceph:v18.2"))
		})

		It("keeps unrelated GHCR namespaces in the existing BOM layout", func() {
			destination, err := installer.PackageImageDestination("ghcr.io/codesphere-cloud-other/api:v1", "registry.example.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("registry.example.com/ghcr_io/codesphere-cloud-other/api:v1"))
		})

		It("leaves references without a registry below their first path segment", func() {
			destination, err := installer.PackageImageDestination(
				"alpine/kubectl:1.35.4",
				"10.10.0.2:5000",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("10.10.0.2:5000/alpine/kubectl:1.35.4"))
		})

		It("leaves a single segment reference unqualified", func() {
			destination, err := installer.PackageImageDestination("alpine:3.21", "10.10.0.2:5000")

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("10.10.0.2:5000/alpine:3.21"))
		})

		It("copies a reference carrying a tag and a digest to its tag", func() {
			destination, err := installer.PackageImageDestination(
				"ghcr.io/codesphere-cloud/api:main@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"10.10.0.2:5000",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("10.10.0.2:5000/codesphere-cloud/api:main"))
		})

		It("preserves digests", func() {
			destination, err := installer.PackageImageDestination(
				"ghcr.io/codesphere-cloud/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"registry.example.com/mirror",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("registry.example.com/mirror/codesphere-cloud/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
		})

		It("rejects an empty destination", func() {
			_, err := installer.PackageImageDestination("ghcr.io/codesphere-cloud/api:v1", "")
			Expect(err).To(MatchError("destination registry must not be empty"))
		})
	})

	Describe("PackageChartDestination", func() {
		It("drops the source registry of a chart", func() {
			destination, err := installer.PackageChartDestination(
				"oci://ghcr.io/codesphere-cloud/charts/pc-applications:0.157.0",
				"10.10.0.5:5000",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("10.10.0.5:5000/codesphere-cloud/charts/pc-applications:0.157.0"))
		})
	})

	Describe("ReadPackageArtifacts", func() {
		var (
			fileIO         *util.MockFileIO
			packageManager *installer.MockPackageManager
		)

		BeforeEach(func() {
			fileIO = util.NewMockFileIO(GinkgoT())
			packageManager = installer.NewMockPackageManager(GinkgoT())
			packageManager.EXPECT().FileIO().Return(fileIO)
			packageManager.EXPECT().GetDependencyPath("bom-index.json").Return("index/location.json")
		})

		It("reads images and charts and prepares a stable transfer plan", func() {
			packageManager.EXPECT().GetDependencyPath("bom.json").Return("legacy/bom.json")
			fileIO.EXPECT().ReadFile("index/location.json").Return(nil, os.ErrNotExist)
			fileIO.EXPECT().ReadFile("legacy/bom.json").Return([]byte(`{
				"components": {
					"codesphere": {
						"containerImages": {"api": "ghcr.io/codesphere-cloud/api:v1"},
						"files": {"chart": {"ociRef": "oci://ghcr.io/codesphere-cloud/charts/app:v1"}}
					}
				}
			}`), nil)

			artifacts, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com/mirror")
			Expect(err).NotTo(HaveOccurred())
			Expect(artifacts).To(Equal([]installer.PackageArtifact{
				{Source: "ghcr.io/codesphere-cloud/api:v1", Destination: "registry.example.com/mirror/codesphere-cloud/api:v1"},
				{Source: "ghcr.io/codesphere-cloud/charts/app:v1", Destination: "registry.example.com/mirror/codesphere-cloud/charts/app:v1"},
			}))
		})

		It("prefers the index and includes unique images and both chart sources", func() {
			fileIO.EXPECT().ReadFile("index/location.json").Return([]byte(`{
				"version": "v3",
				"components": [
					{"name": "app", "version": "2",
					 "containerImages": {"api": {"ref": "ghcr.io/codesphere/api:v2", "sources": ["Deployment/api"]}},
					 "helmCharts": {"dependency": {"ref": "oci://ghcr.io/codesphere/charts/dependency:v2"}},
					 "files": {"chart": {"ociRef": "ghcr.io/codesphere/charts/app:v2"}, "local": {"srcPath": "script.js"}}},
					{"name": "dependency", "version": "2",
					 "containerImages": {"duplicate": {"ref": "ghcr.io/codesphere/api:v2"}, "empty": {"ref": ""}},
					 "helmCharts": {"empty": {"ref": ""}},
					 "files": {"chart": {"ociRef": "ghcr.io/codesphere/charts/dependency:v2"}}}
				]
			}`), nil)

			artifacts, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com/mirror")
			Expect(err).NotTo(HaveOccurred())
			Expect(artifacts).To(Equal([]installer.PackageArtifact{
				{Source: "ghcr.io/codesphere/api:v2", Destination: "registry.example.com/mirror/ghcr_io/codesphere/api:v2"},
				{Source: "ghcr.io/codesphere/charts/app:v2", Destination: "registry.example.com/mirror/codesphere/charts/app:v2"},
				{Source: "ghcr.io/codesphere/charts/dependency:v2", Destination: "registry.example.com/mirror/codesphere/charts/dependency:v2"},
			}))
		})

		It("uses the declared image digest as the source while preserving the destination tag", func() {
			fileIO.EXPECT().ReadFile("index/location.json").Return([]byte(`{
				"version": "v3", "components": [{"name": "app", "version": "2",
				"containerImages": {"api": {"ref": "ghcr.io/codesphere-cloud/api:v2",
				"digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}]
			}`), nil)

			artifacts, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com/mirror")
			Expect(err).NotTo(HaveOccurred())
			Expect(artifacts).To(Equal([]installer.PackageArtifact{{
				Source:      "ghcr.io/codesphere-cloud/api:v2@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Destination: "registry.example.com/mirror/codesphere-cloud/api:v2",
			}}))
		})

		It("reports malformed indexes without falling back", func() {
			fileIO.EXPECT().ReadFile("index/location.json").Return([]byte(`{"components":`), nil)

			_, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com")
			Expect(err).To(MatchError(ContainSubstring("failed to parse JSON BOM index")))
		})

		It("reports unreadable indexes without falling back", func() {
			fileIO.EXPECT().ReadFile("index/location.json").Return(nil, os.ErrPermission)

			_, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com")
			Expect(errors.Is(err, os.ErrPermission)).To(BeTrue())
		})

		It("rejects unsupported index versions without falling back", func() {
			fileIO.EXPECT().ReadFile("index/location.json").Return([]byte(`{"version":"v4","components":[]}`), nil)

			_, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com")
			Expect(err).To(MatchError(ContainSubstring(`unsupported BOM index version "v4"`)))
		})

		It("rejects unsupported component versions without falling back", func() {
			fileIO.EXPECT().ReadFile("index/location.json").Return([]byte(`{
				"version":"v3", "components":[{"name":"app","version":"3"}]
			}`), nil)

			_, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com")
			Expect(err).To(MatchError(ContainSubstring(`unsupported BOM component "app" version "3"`)))
		})

		It("reports a missing legacy BOM when neither file exists", func() {
			packageManager.EXPECT().GetDependencyPath("bom.json").Return("legacy/bom.json")
			fileIO.EXPECT().ReadFile("index/location.json").Return(nil, os.ErrNotExist)
			fileIO.EXPECT().ReadFile("legacy/bom.json").Return(nil, os.ErrNotExist)

			_, err := installer.ReadPackageArtifacts(packageManager, "registry.example.com")
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		})
	})

	Describe("CopyPackageArtifacts", func() {
		It("copies all artifacts in order", func() {
			copier := installer.NewMockArtifactCopier(GinkgoT())
			artifacts := []installer.PackageArtifact{
				{Source: "source/one:v1", Destination: "dest/one:v1"},
				{Source: "source/two:v2", Destination: "dest/two:v2"},
			}

			first := copier.EXPECT().Copy(mock.Anything, "source/one:v1", "dest/one:v1").Return(nil).Once()
			copier.EXPECT().Copy(mock.Anything, "source/two:v2", "dest/two:v2").Return(nil).Once().NotBefore(first)
			Expect(installer.CopyPackageArtifacts(context.Background(), copier, artifacts)).To(Succeed())
		})

		It("adds source and destination context to copy failures", func() {
			copier := installer.NewMockArtifactCopier(GinkgoT())
			copier.EXPECT().Copy(mock.Anything, "source/one:v1", "dest/one:v1").Return(errors.New("denied")).Once()
			err := installer.CopyPackageArtifacts(context.Background(), copier, []installer.PackageArtifact{
				{Source: "source/one:v1", Destination: "dest/one:v1"},
			})

			Expect(err).To(MatchError("failed to copy source/one:v1 to dest/one:v1: denied"))
		})
	})
})
