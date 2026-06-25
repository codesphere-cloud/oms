// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bom_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
)

var _ = Describe("Bom", func() {
	var (
		tempDir   string
		bomFile   string
		sampleBom map[string]interface{}
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "bom_test")
		Expect(err).NotTo(HaveOccurred())

		bomFile = filepath.Join(tempDir, "bom.json")

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
								"exclude": []string{"*.json", "values-*.yaml"},
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
		_ = os.RemoveAll(tempDir)
	})

	Describe("Parse", func() {
		It("should parse a valid BOM file successfully", func() {
			bomData, err := json.Marshal(sampleBom)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(bomFile, bomData, 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := bom.Parse(bomFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Components).To(HaveKey("codesphere"))
			Expect(cfg.Components).To(HaveKey("docker"))
			Expect(cfg.Components).To(HaveKey("kubernetes"))
			Expect(cfg.Migrations.Db.Path).To(Equal("packages/migrations/released"))
			Expect(cfg.Migrations.Db.From).To(Equal("0.0.1"))
		})

		It("should return error for non-existent file", func() {
			_, err := bom.Parse("/non/existent/file.json")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read BOM file"))
		})

		It("should return error for invalid JSON", func() {
			err := os.WriteFile(bomFile, []byte("invalid json content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			_, err = bom.Parse(bomFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse JSON BOM"))
		})

		It("should handle empty BOM file", func() {
			err := os.WriteFile(bomFile, []byte("{}"), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := bom.Parse(bomFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Components).To(BeEmpty())
		})
	})

	Describe("GetCodesphereContainerImages", func() {
		var cfg *bom.Config

		BeforeEach(func() {
			bomData, err := json.Marshal(sampleBom)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(bomFile, bomData, 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err = bom.Parse(bomFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return all codesphere container images", func() {
			images, err := cfg.GetCodesphereContainerImages()
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
			Expect(len(images)).To(Equal(6))
		})

		It("should return error when codesphere component is missing", func() {
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

			freshCfg, err := bom.Parse(bomFile)
			Expect(err).NotTo(HaveOccurred())

			_, err = freshCfg.GetCodesphereContainerImages()
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

			freshCfg, err := bom.Parse(bomFile)
			Expect(err).NotTo(HaveOccurred())

			images, err := freshCfg.GetCodesphereContainerImages()
			Expect(err).NotTo(HaveOccurred())
			Expect(images).To(BeEmpty())
		})
	})

	Describe("GHCRImageReferences", func() {
		It("returns sorted unique GHCR refs from typed BOM fields", func() {
			cfg := &bom.Config{
				Components: map[string]bom.ComponentConfig{
					"gateway": {
						Files: map[string]bom.FileRef{
							"chart": {
								OciRef: "ghcr.io/codesphere-cloud/charts/gateway:0.13.3",
							},
						},
						ContainerImages: map[string]string{
							"envoyProxy":   "ghcr.io/codesphere-cloud/docker/envoyproxy/envoy:distroless-v1.37.0",
							"ingressNginx": "registry.k8s.io/ingress-nginx/controller:v1.13.2",
						},
					},
					"duplicate": {
						Files: map[string]bom.FileRef{
							"chart": {
								OciRef: "docker://ghcr.io/codesphere-cloud/charts/gateway:0.13.3",
							},
						},
						ContainerImages: map[string]string{
							"missingTag": "ghcr.io/codesphere-cloud/docker/alpine/kubectl",
						},
					},
				},
			}

			Expect(cfg.GHCRImageReferences()).To(Equal([]string{
				"ghcr.io/codesphere-cloud/charts/gateway:0.13.3",
				"ghcr.io/codesphere-cloud/docker/envoyproxy/envoy:distroless-v1.37.0",
			}))
		})
	})
})
