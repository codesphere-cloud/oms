// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("Config", func() {
	var (
		config *installer.Config
	)

	BeforeEach(func() {
		config = installer.NewConfig()
	})

	Describe("NewConfig", func() {
		It("creates a new Config with FilesystemWriter", func() {
			newConfig := installer.NewConfig()
			Expect(newConfig).ToNot(BeNil())
			Expect(newConfig.FileIO).ToNot(BeNil())
			Expect(newConfig.FileIO).To(BeAssignableToTypeOf(&util.FilesystemWriter{}))
		})
	})

	Describe("ParseConfigYaml", func() {
		Context("when config file exists and is valid", func() {
			It("successfully parses the configuration", func() {
				tempDir := GinkgoT().TempDir()
				tempConfigFile := filepath.Join(tempDir, "config.yaml")

				validConfigContent := `
registry:
  server: "registry.example.com"
codesphere:
  deployConfig:
    images:
      ubuntu-24.04:
        name: "ubuntu-24.04"
        supportedUntil: "2025-12-31"
        flavors:
          default:
            image:
              bomRef: "ubuntu:24.04"
              dockerfile: "Dockerfile"
`
				err := os.WriteFile(tempConfigFile, []byte(validConfigContent), 0644)
				Expect(err).ToNot(HaveOccurred())

				rootConfig, err := config.ParseConfigYaml(tempConfigFile)

				Expect(err).ToNot(HaveOccurred())
				Expect(rootConfig.Registry.Server).To(Equal("registry.example.com"))
				Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("ubuntu-24.04"))
			})
		})

		Context("when config file does not exist", func() {
			It("returns an error", func() {
				nonExistentFile := "/path/to/nonexistent/config.yaml"

				_, err := config.ParseConfigYaml(nonExistentFile)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse config.yaml"))
			})
		})

		Context("when config file has invalid YAML", func() {
			It("returns an error", func() {
				tempDir := GinkgoT().TempDir()
				tempConfigFile := filepath.Join(tempDir, "invalid-config.yaml")

				invalidConfigContent := `
registry:
  server: "registry.example.com"
  username: "testuser"
  password: "testpass"
codesphere:
  deploy_config:
    images:
      ubuntu-24.04:
        name: "ubuntu-24.04"
        supported_until: "2025-12-31"
        flavors:
          default:
            image:
              bom_ref: "ubuntu:24.04"
              dockerfile: "Dockerfile"
invalid_yaml: [unclosed_bracket
`
				err := os.WriteFile(tempConfigFile, []byte(invalidConfigContent), 0644)
				Expect(err).ToNot(HaveOccurred())

				_, err = config.ParseConfigYaml(tempConfigFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse config.yaml"))
			})
		})

		Context("when config file is empty", func() {
			It("returns empty config without error", func() {
				tempDir := GinkgoT().TempDir()
				tempConfigFile := filepath.Join(tempDir, "empty-config.yaml")

				err := os.WriteFile(tempConfigFile, []byte(""), 0644)
				Expect(err).ToNot(HaveOccurred())

				rootConfig, err := config.ParseConfigYaml(tempConfigFile)

				Expect(err).ToNot(HaveOccurred())
				Expect(rootConfig.Registry.Server).To(BeEmpty())
				Expect(rootConfig.Codesphere.DeployConfig.Images).To(BeEmpty())
			})
		})
	})

	Describe("ConfigManager interface", func() {
		It("implements ConfigManager interface", func() {
			var configManager installer.ConfigManager = config
			Expect(configManager).ToNot(BeNil())
		})

		It("has all required methods", func() {
			var configManager installer.ConfigManager = config

			// Test that methods exist by calling them with invalid parameters
			// and checking that we get errors (proving the methods are callable)
			_, err1 := configManager.ParseConfigYaml("/nonexistent/path")
			Expect(err1).To(HaveOccurred())
		})
	})

	Describe("Config struct fields", func() {
		It("has FileIO field", func() {
			Expect(config.FileIO).ToNot(BeNil())
		})

		It("allows FileIO to be replaced with mock", func() {
			mockFileIO := util.NewMockFileIO(GinkgoT())
			config.FileIO = mockFileIO
			Expect(config.FileIO).To(Equal(mockFileIO))
		})
	})

	Describe("Error handling and edge cases", func() {
		Context("ParseConfigYaml with various file permissions", func() {
			It("handles file with read permissions", func() {
				tempDir := GinkgoT().TempDir()
				tempConfigFile := filepath.Join(tempDir, "config.yaml")

				validConfigContent := `
registry:
  server: "registry.example.com"
`
				err := os.WriteFile(tempConfigFile, []byte(validConfigContent), 0644)
				Expect(err).ToNot(HaveOccurred())

				_, err = config.ParseConfigYaml(tempConfigFile)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("ExtractOciImageIndex with various scenarios", func() {
			It("handles empty image file path", func() {
				// Test moved to package_test.go
				Skip("ExtractOciImageIndex tests moved to package_test.go")
			})

			It("handles directory instead of file", func() {
				// Test moved to package_test.go
				Skip("ExtractOciImageIndex tests moved to package_test.go")
			})
		})
	})

	Describe("Integration scenarios", func() {
		Context("full workflow simulation", func() {
			It("can parse complex configuration successfully", func() {
				tempDir := GinkgoT().TempDir()
				configFile := filepath.Join(tempDir, "config.yaml")

				// Create a realistic config file
				configContent := `
registry:
  server: "my-registry.example.com"
codesphere:
  deployConfig:
    images:
      ubuntu-24.04:
        name: "ubuntu-24.04"
        supportedUntil: "2025-12-31"
        flavors:
          default:
            image:
              bomRef: "ubuntu:24.04"
              dockerfile: "Dockerfile"
          minimal:
            image:
              bomRef: "ubuntu:24.04-minimal"
              dockerfile: "Dockerfile.minimal"
      alpine-3.18:
        name: "alpine-3.18"
        supportedUntil: "2024-12-31"
        flavors:
          default:
            image:
              bomRef: "alpine:3.18"
              dockerfile: "Dockerfile.alpine"
`
				err := os.WriteFile(configFile, []byte(configContent), 0644)
				Expect(err).ToNot(HaveOccurred())

				rootConfig, err := config.ParseConfigYaml(configFile)

				Expect(err).ToNot(HaveOccurred())
				Expect(rootConfig.Registry.Server).To(Equal("my-registry.example.com"))

				// Check images
				Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveLen(2))
				Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("ubuntu-24.04"))
				Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("alpine-3.18"))

				// Check ubuntu image details
				ubuntuImage := rootConfig.Codesphere.DeployConfig.Images["ubuntu-24.04"]
				Expect(ubuntuImage.Name).To(Equal("ubuntu-24.04"))
				Expect(ubuntuImage.SupportedUntil).To(Equal("2025-12-31"))
				Expect(ubuntuImage.Flavors).To(HaveLen(2))
				Expect(ubuntuImage.Flavors).To(HaveKey("default"))
				Expect(ubuntuImage.Flavors).To(HaveKey("minimal"))

				// Check alpine image details
				alpineImage := rootConfig.Codesphere.DeployConfig.Images["alpine-3.18"]
				Expect(alpineImage.Name).To(Equal("alpine-3.18"))
				Expect(alpineImage.SupportedUntil).To(Equal("2024-12-31"))
				Expect(alpineImage.Flavors).To(HaveLen(1))
				Expect(alpineImage.Flavors).To(HaveKey("default"))
			})
		})
	})
})
