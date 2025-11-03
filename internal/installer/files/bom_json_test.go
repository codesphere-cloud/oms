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

var _ = Describe("BomJson", func() {
	var (
		bomConfig *files.BomConfig
		tempDir   string
		bomFile   string
		sampleBom map[string]interface{}
	)

	BeforeEach(func() {
		bomConfig = &files.BomConfig{}

		var err error
		tempDir, err = os.MkdirTemp("", "bom_test")
		Expect(err).NotTo(HaveOccurred())

		bomFile = filepath.Join(tempDir, "bom.json")

		// Create sample BOM data matching the real structure
		sampleBom = map[string]interface{}{
			"components": map[string]interface{}{
				"codesphere": map[string]interface{}{
					"containerImages": map[string]interface{}{
						"workspace-agent-24.04": "ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-24.04:codesphere-v1.66.0",
						"workspace-agent-20.04": "ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-20.04:codesphere-v1.66.0",
						"auth-service":          "ghcr.io/codesphere-cloud/codesphere-monorepo/auth-service:codesphere-v1.66.0",
						"ide-service":           "ghcr.io/codesphere-cloud/codesphere-monorepo/ide-service:codesphere-v1.66.0",
						"workspace-service":     "ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-service:codesphere-v1.66.0",
						"nginx":                 "ghcr.io/codesphere-cloud/docker/nginx:1.26.3",
					},
					"files": map[string]interface{}{
						"chart": map[string]interface{}{
							"glob": map[string]interface{}{
								"cwd":     "helm/codesphere",
								"include": "**/*",
								"exclude": []string{"*.json5", "values-*.yaml"},
							},
						},
						"schemaDump": map[string]interface{}{
							"srcPath": "infra/bin/private-cloud/pg-masterdata.sql",
						},
					},
				},
				"docker": map[string]interface{}{
					"files": map[string]interface{}{
						"24.04_containerd": map[string]interface{}{
							"srcUrl":     "https://download.docker.com/linux/ubuntu/dists/noble/pool/stable/amd64/containerd.io_1.6.31-1_amd64.deb",
							"executable": false,
						},
					},
				},
				"kubernetes": map[string]interface{}{
					"files": map[string]interface{}{
						"k0s": map[string]interface{}{
							"srcUrl":     "https://github.com/k0sproject/k0s/releases/download/v1.28.4%2Bk0s.0/k0s-v1.28.4+k0s.0-amd64",
							"executable": true,
						},
					},
				},
			},
			"migrations": map[string]interface{}{
				"db": map[string]interface{}{
					"path": "packages/migrations/released",
					"from": "0.0.1",
				},
			},
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("ParseBomConfig", func() {
		It("should parse a valid BOM file successfully", func() {
			// Write sample BOM to file
			bomData, err := json.Marshal(sampleBom)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(bomFile, bomData, 0644)
			Expect(err).NotTo(HaveOccurred())

			err = bomConfig.ParseBomConfig(bomFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(bomConfig.Components).To(HaveKey("codesphere"))
			Expect(bomConfig.Components).To(HaveKey("docker"))
			Expect(bomConfig.Components).To(HaveKey("kubernetes"))
			Expect(bomConfig.Migrations.Db.Path).To(Equal("packages/migrations/released"))
			Expect(bomConfig.Migrations.Db.From).To(Equal("0.0.1"))
		})

		It("should return error for non-existent file", func() {
			err := bomConfig.ParseBomConfig("/non/existent/file.json")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read BOM file"))
		})

		It("should return error for invalid JSON", func() {
			err := os.WriteFile(bomFile, []byte("invalid json content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = bomConfig.ParseBomConfig(bomFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse JSON BOM"))
		})

		It("should handle empty BOM file", func() {
			err := os.WriteFile(bomFile, []byte("{}"), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = bomConfig.ParseBomConfig(bomFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(bomConfig.Components).To(BeEmpty())
		})
	})

	Describe("GetCodesphereContainerImages", func() {
		BeforeEach(func() {
			bomData, err := json.Marshal(sampleBom)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(bomFile, bomData, 0644)
			Expect(err).NotTo(HaveOccurred())

			err = bomConfig.ParseBomConfig(bomFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return all codesphere container images", func() {
			images, err := bomConfig.GetCodesphereContainerImages()
			Expect(err).NotTo(HaveOccurred())
			Expect(images).NotTo(BeNil())

			Expect(images).To(HaveKey("workspace-agent-24.04"))
			Expect(images).To(HaveKey("workspace-agent-20.04"))
			Expect(images).To(HaveKey("auth-service"))
			Expect(images).To(HaveKey("ide-service"))
			Expect(images).To(HaveKey("workspace-service"))
			Expect(images).To(HaveKey("nginx"))

			Expect(images["workspace-agent-24.04"]).To(Equal("ghcr.io/codesphere-cloud/codesphere-monorepo/workspace-agent-24.04:codesphere-v1.66.0"))
			Expect(images["auth-service"]).To(Equal("ghcr.io/codesphere-cloud/codesphere-monorepo/auth-service:codesphere-v1.66.0"))

			// Should have 6 images in our test data
			Expect(len(images)).To(Equal(6))
		})

		It("should return error when codesphere component is missing", func() {
			// Create a fresh bomConfig for this test to avoid state from BeforeEach
			freshBomConfig := &files.BomConfig{}

			// Create BOM without codesphere component
			bomWithoutCodesphere := map[string]interface{}{
				"components": map[string]interface{}{
					"docker": map[string]interface{}{
						"files": map[string]interface{}{
							"containerd": map[string]interface{}{
								"srcUrl": "https://download.docker.com/test.deb",
							},
						},
					},
				},
			}

			bomData, err := json.Marshal(bomWithoutCodesphere)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(bomFile, bomData, 0644)
			Expect(err).NotTo(HaveOccurred())

			err = freshBomConfig.ParseBomConfig(bomFile)
			Expect(err).NotTo(HaveOccurred())

			_, err = freshBomConfig.GetCodesphereContainerImages()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("codesphere component not found in BOM"))
		})

		It("should handle codesphere component with no container images", func() {
			bomWithEmptyCodesphere := map[string]interface{}{
				"components": map[string]interface{}{
					"codesphere": map[string]interface{}{
						"files": map[string]interface{}{
							"chart": map[string]interface{}{
								"glob": map[string]interface{}{
									"cwd":     "helm/codesphere",
									"include": "**/*",
								},
							},
						},
					},
				},
			}

			bomData, err := json.Marshal(bomWithEmptyCodesphere)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(bomFile, bomData, 0644)
			Expect(err).NotTo(HaveOccurred())

			err = bomConfig.ParseBomConfig(bomFile)
			Expect(err).NotTo(HaveOccurred())

			images, err := bomConfig.GetCodesphereContainerImages()
			Expect(err).NotTo(HaveOccurred())
			Expect(images).To(BeEmpty())
		})
	})
})
