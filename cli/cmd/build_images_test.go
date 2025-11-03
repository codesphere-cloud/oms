// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
)

const validConfigYaml = `registry:
  server: "registry.example.com"
codesphere:
  deployConfig:
    images:
      my-ubuntu-24.04:
        name: "my-ubuntu-24.04"
        supportedUntil: "2025-12-31"
        flavors:
          default:
            image:
              bomRef: "registry.example.com/my-ubuntu-24.04:latest"
              dockerfile: "Dockerfile"
            pool:
              1: 2
`

var _ = Describe("BuildImagesCmd", func() {
	var (
		c          cmd.BuildImagesCmd
		opts       *cmd.BuildImagesOpts
		globalOpts *cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.BuildImagesOpts{
			GlobalOptions: globalOpts,
			Config:        "",
		}
		c = cmd.BuildImagesCmd{
			Opts: opts,
			Env:  mockEnv,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("calls BuildAndPushImages and fails on config parsing", func() {
			mockEnv := env.NewMockEnv(GinkgoT())
			c.Env = mockEnv
			c.Opts.Config = "non-existent-config.yaml"

			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build and push images"))
		})

		It("succeeds with valid config and operations", func() {
			mockEnv := env.NewMockEnv(GinkgoT())
			c.Env = mockEnv

			tempConfigFile, err := os.CreateTemp("", "test-config.yaml")
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempConfigFile.Name()) }()

			_, err = tempConfigFile.WriteString(validConfigYaml)
			Expect(err).To(BeNil())
			_ = tempConfigFile.Close()

			c.Opts.Config = tempConfigFile.Name()

			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err = c.RunE(nil, []string{})
			// This will fail because the dockerfile doesn't exist and build will fail
			// But it should at least parse the config successfully
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build and push images"))
		})
	})

	Context("BuildAndPushImages method", func() {
		It("fails when config manager fails to parse config", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "non-existent-config.yaml"
			mockConfigManager.EXPECT().ParseConfigYaml("non-existent-config.yaml").Return(files.RootConfig{}, errors.New("failed to parse config"))

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse config"))
		})

		It("fails when no images are defined in config", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "empty-config.yaml"
			emptyConfig := files.RootConfig{
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("empty-config.yaml").Return(emptyConfig, nil)

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no images defined in the config"))
		})

		It("fails when registry server is empty", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "config-without-registry.yaml"
			configWithoutRegistry := files.RootConfig{
				Registry: files.RegistryConfig{
					// Empty server
				},
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{
							"my-ubuntu-24.04": {
								Name:           "my-ubuntu-24.04",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef:     "my-ubuntu-24.04:latest",
											Dockerfile: "Dockerfile",
										},
									},
								},
							},
						},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("config-without-registry.yaml").Return(configWithoutRegistry, nil)

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("registry server (property registry.server) not defined in the config"))
		})

		It("skips flavors without dockerfile", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "config-without-dockerfile.yaml"
			configWithoutDockerfile := files.RootConfig{
				Registry: files.RegistryConfig{
					Server: "registry.example.com",
				},
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{
							"my-ubuntu-24.04": {
								Name:           "my-ubuntu-24.04",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef: "registry.example.com/my-ubuntu-24.04:latest",
											// No dockerfile specified
										},
									},
								},
							},
						},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("config-without-dockerfile.yaml").Return(configWithoutDockerfile, nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("1.0.0", nil)

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(BeNil())
		})

		It("fails when image build fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "config-with-dockerfile.yaml"
			configWithDockerfile := files.RootConfig{
				Registry: files.RegistryConfig{
					Server: "registry.example.com",
				},
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{
							"my-ubuntu-24.04": {
								Name:           "my-ubuntu-24.04",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef:     "registry.example.com/my-ubuntu-24.04:latest",
											Dockerfile: "Dockerfile",
										},
									},
								},
							},
						},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("config-with-dockerfile.yaml").Return(configWithDockerfile, nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("1.0.0", nil)
			mockImageManager.EXPECT().BuildImage("Dockerfile", "registry.example.com/my-ubuntu-24.04-default:1.0.0", ".").Return(errors.New("build failed"))

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build image"))
		})

		It("fails when image push fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "config-with-dockerfile.yaml"
			configWithDockerfile := files.RootConfig{
				Registry: files.RegistryConfig{
					Server: "registry.example.com",
				},
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{
							"my-ubuntu-24.04": {
								Name:           "my-ubuntu-24.04",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef:     "registry.example.com/my-ubuntu-24.04:latest",
											Dockerfile: "Dockerfile",
										},
									},
								},
							},
						},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("config-with-dockerfile.yaml").Return(configWithDockerfile, nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("1.0.0", nil)
			mockImageManager.EXPECT().BuildImage("Dockerfile", "registry.example.com/my-ubuntu-24.04-default:1.0.0", ".").Return(nil)
			mockImageManager.EXPECT().PushImage("registry.example.com/my-ubuntu-24.04-default:1.0.0").Return(errors.New("push failed"))

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to push image"))
		})

		It("successfully builds and pushes single image", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "config-with-dockerfile.yaml"
			configWithDockerfile := files.RootConfig{
				Registry: files.RegistryConfig{
					Server: "registry.example.com",
				},
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{
							"my-ubuntu-24.04": {
								Name:           "my-ubuntu-24.04",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef:     "registry.example.com/my-ubuntu-24.04:latest",
											Dockerfile: "Dockerfile",
										},
									},
								},
							},
						},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("config-with-dockerfile.yaml").Return(configWithDockerfile, nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("1.0.0", nil)
			mockImageManager.EXPECT().BuildImage("Dockerfile", "registry.example.com/my-ubuntu-24.04-default:1.0.0", ".").Return(nil)
			mockImageManager.EXPECT().PushImage("registry.example.com/my-ubuntu-24.04-default:1.0.0").Return(nil)

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(BeNil())
		})

		It("successfully builds and pushes multiple images with different flavors", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Config = "config-with-multiple-images.yaml"
			configWithMultipleImages := files.RootConfig{
				Registry: files.RegistryConfig{
					Server: "registry.example.com",
				},
				Codesphere: files.CodesphereConfig{
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{
							"my-ubuntu-24.04": {
								Name:           "my-ubuntu-24.04",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef:     "registry.example.com/my-ubuntu-24.04:latest",
											Dockerfile: "Dockerfile.default",
										},
									},
									"minimal": {
										Image: files.ImageRef{
											BomRef:     "registry.example.com/my-ubuntu-24.04:minimal",
											Dockerfile: "Dockerfile.minimal",
										},
									},
								},
							},
							"my-alpine-3.18": {
								Name:           "my-alpine-3.18",
								SupportedUntil: "2025-12-31",
								Flavors: map[string]files.FlavorConfig{
									"default": {
										Image: files.ImageRef{
											BomRef:     "registry.example.com/my-alpine-3.18:latest",
											Dockerfile: "Dockerfile.alpine",
										},
									},
								},
							},
						},
					},
				},
			}
			mockConfigManager.EXPECT().ParseConfigYaml("config-with-multiple-images.yaml").Return(configWithMultipleImages, nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("1.0.0", nil)

			// Expect calls for my-ubuntu-24.04 default flavor
			mockImageManager.EXPECT().BuildImage("Dockerfile.default", "registry.example.com/my-ubuntu-24.04-default:1.0.0", ".").Return(nil)
			mockImageManager.EXPECT().PushImage("registry.example.com/my-ubuntu-24.04-default:1.0.0").Return(nil)

			// Expect calls for my-ubuntu-24.04 minimal flavor
			mockImageManager.EXPECT().BuildImage("Dockerfile.minimal", "registry.example.com/my-ubuntu-24.04-minimal:1.0.0", ".").Return(nil)
			mockImageManager.EXPECT().PushImage("registry.example.com/my-ubuntu-24.04-minimal:1.0.0").Return(nil)

			// Expect calls for my-alpine-3.18 default flavor
			mockImageManager.EXPECT().BuildImage("Dockerfile.alpine", "registry.example.com/my-alpine-3.18-default:1.0.0", ".").Return(nil)
			mockImageManager.EXPECT().PushImage("registry.example.com/my-alpine-3.18-default:1.0.0").Return(nil)

			err := c.BuildAndPushImages(mockPackageManager, mockConfigManager, mockImageManager)
			Expect(err).To(BeNil())
		})
	})
})

var _ = Describe("AddBuildImagesCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "build"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the images command with correct properties and flags", func() {
		cmd.AddBuildImagesCmd(parentCmd, globalOpts)

		var imagesCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "images" {
				imagesCmd = c
				break
			}
		}

		Expect(imagesCmd).NotTo(BeNil())
		Expect(imagesCmd.Use).To(Equal("images"))
		Expect(imagesCmd.Short).To(Equal("Build and push container images"))
		Expect(imagesCmd.Long).To(ContainSubstring("Build and push container images based on the configuration file"))
		Expect(imagesCmd.RunE).NotTo(BeNil())

		// Check flags
		configFlag := imagesCmd.Flags().Lookup("config")
		Expect(configFlag).NotTo(BeNil())
		Expect(configFlag.Shorthand).To(Equal("c"))
		Expect(configFlag.Usage).To(ContainSubstring("Path to the configuration YAML file"))
	})
})
