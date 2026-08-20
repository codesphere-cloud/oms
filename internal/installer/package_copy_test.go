// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recordingArtifactCopier struct {
	copies []installer.PackageArtifact
	err    error
}

func (c *recordingArtifactCopier) Copy(_ context.Context, source, destination string) error {
	c.copies = append(c.copies, installer.PackageArtifact{Source: source, Destination: destination})
	return c.err
}

var _ = Describe("Package artifact copying", func() {
	Describe("PackageArtifactDestination", func() {
		It("preserves the source repository path and tag below the destination", func() {
			destination, err := installer.PackageArtifactDestination(
				"oci://ghcr.io/codesphere-cloud/charts/pc-apps:1.2.3",
				"oci://registry.example.com/private-cloud/",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("registry.example.com/private-cloud/codesphere-cloud/charts/pc-apps:1.2.3"))
		})

		It("preserves digests", func() {
			destination, err := installer.PackageArtifactDestination(
				"ghcr.io/codesphere-cloud/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"registry.example.com/mirror",
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(destination).To(Equal("registry.example.com/mirror/codesphere-cloud/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
		})

		It("rejects an empty destination", func() {
			_, err := installer.PackageArtifactDestination("ghcr.io/codesphere/api:v1", "")
			Expect(err).To(MatchError("destination registry must not be empty"))
		})
	})

	Describe("ReadPackageArtifacts", func() {
		It("reads images and charts and prepares a stable transfer plan", func() {
			tempDir := GinkgoT().TempDir()
			bomPath := filepath.Join(tempDir, "bom.json")
			Expect(os.WriteFile(bomPath, []byte(`{
				"components": {
					"codesphere": {
						"containerImages": {"api": "ghcr.io/codesphere/api:v1"},
						"files": {"chart": {"ociRef": "oci://ghcr.io/codesphere/charts/app:v1"}}
					}
				}
			}`), 0644)).To(Succeed())

			artifacts, err := installer.ReadPackageArtifacts(bomPath, "registry.example.com/mirror")
			Expect(err).NotTo(HaveOccurred())
			Expect(artifacts).To(Equal([]installer.PackageArtifact{
				{Source: "ghcr.io/codesphere/api:v1", Destination: "registry.example.com/mirror/codesphere/api:v1"},
				{Source: "ghcr.io/codesphere/charts/app:v1", Destination: "registry.example.com/mirror/codesphere/charts/app:v1"},
			}))
		})
	})

	Describe("CopyPackageArtifacts", func() {
		It("copies all artifacts in order", func() {
			copier := &recordingArtifactCopier{}
			artifacts := []installer.PackageArtifact{
				{Source: "source/one:v1", Destination: "dest/one:v1"},
				{Source: "source/two:v2", Destination: "dest/two:v2"},
			}

			Expect(installer.CopyPackageArtifacts(context.Background(), copier, artifacts)).To(Succeed())
			Expect(copier.copies).To(Equal(artifacts))
		})

		It("adds source and destination context to copy failures", func() {
			copier := &recordingArtifactCopier{err: errors.New("denied")}
			err := installer.CopyPackageArtifacts(context.Background(), copier, []installer.PackageArtifact{
				{Source: "source/one:v1", Destination: "dest/one:v1"},
			})

			Expect(err).To(MatchError("failed to copy source/one:v1 to dest/one:v1: denied"))
		})
	})
})
