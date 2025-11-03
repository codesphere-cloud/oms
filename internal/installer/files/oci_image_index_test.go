// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("OciImageIndex", func() {
	var (
		ociIndex    *files.OCIImageIndex
		tempDir     string
		indexFile   string
		sampleIndex map[string]interface{}
	)

	BeforeEach(func() {
		ociIndex = &files.OCIImageIndex{}

		var err error
		tempDir, err = os.MkdirTemp("", "oci_index_test")
		Expect(err).NotTo(HaveOccurred())

		indexFile = filepath.Join(tempDir, "index.json")

		// Create sample OCI Image Index data matching the real structure
		sampleIndex = map[string]interface{}{
			"schemaVersion": 2,
			"mediaType":     "application/vnd.oci.image.index.v1+json",
			"manifests": []interface{}{
				map[string]interface{}{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"digest":    "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
					"size":      int64(527),
					"annotations": map[string]interface{}{
						"io.containerd.image.name": "ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-24.04:codesphere-v1.66.0",
					},
				},
				map[string]interface{}{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"digest":    "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
					"size":      int64(842),
					"annotations": map[string]interface{}{
						"io.containerd.image.name": "ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-20.04:codesphere-v1.66.0",
					},
				},
				map[string]interface{}{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"digest":    "sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
					"size":      int64(1024),
					"annotations": map[string]interface{}{
						"io.containerd.image.name": "registry.example.com/auth-service:v1.0.0",
					},
				},
				map[string]interface{}{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"digest":    "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					"size":      int64(256),
					"annotations": map[string]interface{}{
						"org.opencontainers.image.ref.name": "nginx:latest",
						"custom.annotation":                 "some-value",
					},
				},
			},
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("ParseOCIImageConfig", func() {
		It("should parse a valid OCI Image Index file successfully", func() {
			// Write sample index to file
			indexData, err := json.Marshal(sampleIndex)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(indexFile, indexData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// The function expects a file path and looks for index.json in the same directory
			err = ociIndex.ParseOCIImageConfig(indexFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(ociIndex.SchemaVersion).To(Equal(2))
			Expect(ociIndex.MediaType).To(Equal("application/vnd.oci.image.index.v1+json"))
			Expect(ociIndex.Manifests).To(HaveLen(4))

			// Check first manifest entry
			firstManifest := ociIndex.Manifests[0]
			Expect(firstManifest.MediaType).To(Equal("application/vnd.oci.image.manifest.v1+json"))
			Expect(firstManifest.Digest).To(Equal("sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"))
			Expect(firstManifest.Size).To(Equal(int64(527)))
			Expect(firstManifest.Annotations).To(HaveKeyWithValue("io.containerd.image.name", "ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-24.04:codesphere-v1.66.0"))
		})

		It("should return error for non-existent index.json file", func() {
			nonExistentFile := filepath.Join(tempDir, "nonexistent.json")
			err := ociIndex.ParseOCIImageConfig(nonExistentFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to open file"))
			Expect(err.Error()).To(ContainSubstring("index.json"))
		})

		It("should return error for invalid JSON in index.json", func() {
			err := os.WriteFile(indexFile, []byte("invalid json content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = ociIndex.ParseOCIImageConfig(indexFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to decode file"))
		})

		It("should handle empty index.json file", func() {
			err := os.WriteFile(indexFile, []byte("{}"), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = ociIndex.ParseOCIImageConfig(indexFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(ociIndex.SchemaVersion).To(Equal(0))
			Expect(ociIndex.MediaType).To(BeEmpty())
			Expect(ociIndex.Manifests).To(BeEmpty())
		})

		It("should handle index.json in subdirectory when given file path in subdirectory", func() {
			subDir := filepath.Join(tempDir, "subdir")
			err := os.MkdirAll(subDir, 0755)
			Expect(err).NotTo(HaveOccurred())

			subIndexFile := filepath.Join(subDir, "index.json")
			someOtherFile := filepath.Join(subDir, "other.json")

			indexData, err := json.Marshal(sampleIndex)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(subIndexFile, indexData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Pass the "other.json" file path - function should still find index.json in same directory
			err = ociIndex.ParseOCIImageConfig(someOtherFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(ociIndex.SchemaVersion).To(Equal(2))
			Expect(ociIndex.Manifests).To(HaveLen(4))
		})
	})

	Describe("ExtractImageNames", func() {
		BeforeEach(func() {
			indexData, err := json.Marshal(sampleIndex)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(indexFile, indexData, 0644)
			Expect(err).NotTo(HaveOccurred())

			err = ociIndex.ParseOCIImageConfig(indexFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return empty slice when no manifests exist", func() {
			emptyIndex := &files.OCIImageIndex{}
			names, err := emptyIndex.ExtractImageNames()
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(BeEmpty())
		})

		It("should handle manifests without annotations", func() {
			// Create a fresh ociIndex for this test to avoid state from BeforeEach
			freshIndex := &files.OCIImageIndex{}

			indexWithoutAnnotations := map[string]interface{}{
				"schemaVersion": 2,
				"mediaType":     "application/vnd.oci.image.index.v1+json",
				"manifests": []interface{}{
					map[string]interface{}{
						"mediaType": "application/vnd.oci.image.manifest.v1+json",
						"digest":    "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
						"size":      int64(527),
					},
				},
			}

			indexData, err := json.Marshal(indexWithoutAnnotations)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(indexFile, indexData, 0644)
			Expect(err).NotTo(HaveOccurred())

			err = freshIndex.ParseOCIImageConfig(indexFile)
			Expect(err).NotTo(HaveOccurred())

			names, err := freshIndex.ExtractImageNames()
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(BeEmpty())
		})

		It("should extract all image names from manifests with containerd annotations", func() {
			names, err := ociIndex.ExtractImageNames()
			Expect(err).NotTo(HaveOccurred())
			Expect(names).NotTo(BeEmpty())

			Expect(names).To(ContainElement("ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-24.04:codesphere-v1.66.0"))
			Expect(names).To(ContainElement("ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-20.04:codesphere-v1.66.0"))
			Expect(names).To(ContainElement("registry.example.com/auth-service:v1.0.0"))
			Expect(len(names)).To(Equal(3))
		})
	})
})
