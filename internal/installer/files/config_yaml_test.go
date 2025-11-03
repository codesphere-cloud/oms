// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("ConfigYaml", func() {
	var (
		rootConfig *files.RootConfig
		tempDir    string
		configFile string
		sampleYaml string
	)

	BeforeEach(func() {
		rootConfig = &files.RootConfig{}

		var err error
		tempDir, err = os.MkdirTemp("", "config_yaml_test")
		Expect(err).NotTo(HaveOccurred())

		configFile = filepath.Join(tempDir, "config.yaml")

		sampleYaml = `registry:
  server: registry.example.com

codesphere:
  deployConfig:
    images:
      workspace-agent-24.04:
        name: ubuntu-24.04
        supportedUntil: "2029-04-01"
        flavors:
          default:
            image:
              bomRef: workspace-agent-24.04
              dockerfile: dockerfile-24.04
            pool:
              8: 2
              16: 1
          minimal:
            image:
              bomRef: workspace-agent-24.04-minimal
              dockerfile: dockerfile-24.04-minimal
            pool:
              4: 1
      workspace-agent-20.04:
        name: ubuntu-20.04
        supportedUntil: "2025-04-01"
        flavors:
          default:
            image:
              bomRef: workspace-agent-20.04
              dockerfile: dockerfile-20.04
            pool:
              8: 1
      ide-service:
        name: ide-service
        supportedUntil: "2026-01-01"
        flavors:
          default:
            image:
              bomRef: ide-service
            pool:
              4: 2
`
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("ParseConfig", func() {
		It("should parse a valid YAML config file successfully", func() {
			err := os.WriteFile(configFile, []byte(sampleYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.ParseConfig(configFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(Equal("registry.example.com"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("workspace-agent-24.04"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("workspace-agent-20.04"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("ide-service"))

			// Check specific image config
			workspaceAgent24 := rootConfig.Codesphere.DeployConfig.Images["workspace-agent-24.04"]
			Expect(workspaceAgent24.Name).To(Equal("ubuntu-24.04"))
			Expect(workspaceAgent24.SupportedUntil).To(Equal("2029-04-01"))
			Expect(workspaceAgent24.Flavors).To(HaveKey("default"))
			Expect(workspaceAgent24.Flavors).To(HaveKey("minimal"))

			// Check flavor details
			defaultFlavor := workspaceAgent24.Flavors["default"]
			Expect(defaultFlavor.Image.BomRef).To(Equal("workspace-agent-24.04"))
			Expect(defaultFlavor.Image.Dockerfile).To(Equal("dockerfile-24.04"))
			Expect(defaultFlavor.Pool).To(HaveKeyWithValue(8, 2))
			Expect(defaultFlavor.Pool).To(HaveKeyWithValue(16, 1))
		})

		It("should return error for non-existent file", func() {
			err := rootConfig.ParseConfig("/non/existent/config.yaml")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("should return error for invalid YAML", func() {
			invalidYaml := `registry:
  server: registry.example.com
codesphere:
  deployConfig:
    images:
      - invalid: yaml structure without proper mapping
`
			err := os.WriteFile(configFile, []byte(invalidYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.ParseConfig(configFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse YAML config"))
		})

		It("should handle empty config file", func() {
			err := os.WriteFile(configFile, []byte(""), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.ParseConfig(configFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(BeEmpty())
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(BeEmpty())
		})

		It("should handle minimal valid config", func() {
			minimalYaml := `registry:
  server: minimal.registry.com
codesphere:
  deployConfig:
    images: {}
`
			err := os.WriteFile(configFile, []byte(minimalYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.ParseConfig(configFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(Equal("minimal.registry.com"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(BeEmpty())
		})
	})

	Describe("ExtractBomRefs", func() {
		BeforeEach(func() {
			err := os.WriteFile(configFile, []byte(sampleYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.ParseConfig(configFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should extract all BOM references from config", func() {
			bomRefs := rootConfig.ExtractBomRefs()

			Expect(bomRefs).NotTo(BeEmpty())
			Expect(bomRefs).To(ContainElement("workspace-agent-24.04"))
			Expect(bomRefs).To(ContainElement("workspace-agent-24.04-minimal"))
			Expect(bomRefs).To(ContainElement("workspace-agent-20.04"))
			Expect(bomRefs).To(ContainElement("ide-service"))
			Expect(len(bomRefs)).To(Equal(4))
		})

		It("should return empty slice when no images are configured", func() {
			emptyConfig := &files.RootConfig{}
			bomRefs := emptyConfig.ExtractBomRefs()

			Expect(bomRefs).To(BeEmpty())
		})

		It("should handle flavors without BOM references", func() {
			noImagesConfig := &files.RootConfig{}
			yamlWithoutBomRefs := `registry:
  server: registry.example.com
codesphere:
  deployConfig:
    images:
      test-image:
        name: test
        flavors:
          default:
            image:
              dockerfile: dockerfile-only
            pool:
              4: 1
`
			err := os.WriteFile(configFile, []byte(yamlWithoutBomRefs), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = noImagesConfig.ParseConfig(configFile)
			Expect(err).NotTo(HaveOccurred())

			bomRefs := noImagesConfig.ExtractBomRefs()
			Expect(bomRefs).To(BeEmpty())
		})
	})

	Describe("ExtractWorkspaceDockerfiles", func() {
		BeforeEach(func() {
			err := os.WriteFile(configFile, []byte(sampleYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.ParseConfig(configFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return empty map when no images are configured", func() {
			emptyConfig := &files.RootConfig{}
			dockerfiles := emptyConfig.ExtractWorkspaceDockerfiles()

			Expect(dockerfiles).To(BeEmpty())
		})

		It("should extract all Dockerfile paths mapped to their BOM references", func() {
			dockerfiles := rootConfig.ExtractWorkspaceDockerfiles()

			Expect(dockerfiles).NotTo(BeEmpty())
			Expect(dockerfiles).To(HaveKeyWithValue("dockerfile-24.04", "workspace-agent-24.04"))
			Expect(dockerfiles).To(HaveKeyWithValue("dockerfile-24.04-minimal", "workspace-agent-24.04-minimal"))
			Expect(dockerfiles).To(HaveKeyWithValue("dockerfile-20.04", "workspace-agent-20.04"))

			// Should have 3 dockerfile mappings (ide-service has no dockerfile)
			Expect(len(dockerfiles)).To(Equal(3))
		})
	})
})
