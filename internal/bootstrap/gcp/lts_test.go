// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LTS Compatibility", func() {
	Describe("FindLTSSpec", func() {
		It("returns a spec for the LTS 1.77.2 version string", func() {
			spec := gcp.FindLTSSpec("codesphere-lts-v1.77.2")
			Expect(spec).NotTo(BeNil())
			Expect(spec.InstallVersion).To(Equal("codesphere-lts-v1.77.2"))
			Expect(spec.RequiresJumpboxFiles).To(BeTrue())
			Expect(spec.RequiresOmsBinaryUpdate).To(BeTrue())
			Expect(spec.ClearManagedServices).To(BeTrue())
		})

		It("returns nil for another LTS version", func() {
			Expect(gcp.FindLTSSpec("codesphere-lts-v1.80.0")).To(BeNil())
		})

		It("returns nil for an empty string", func() {
			Expect(gcp.FindLTSSpec("")).To(BeNil())
		})

		It("returns nil for a non-LTS version", func() {
			Expect(gcp.FindLTSSpec("master")).To(BeNil())
		})

		It("returns nil for a partial match", func() {
			Expect(gcp.FindLTSSpec("codesphere-lts-v1.77.2-extra")).To(BeNil())
		})
	})

	Describe("FilterExperiments", func() {
		It("keeps only allowed experiments", func() {
			input := []string{"managed-services", "custom-service-image", "secret-management", "ms-in-ls", "sub-path-mount"}
			allowed := []string{"managed-services", "custom-service-image", "ms-in-ls"}
			result := gcp.FilterExperiments(input, allowed)
			Expect(result).To(ConsistOf("managed-services", "custom-service-image", "ms-in-ls"))
		})

		It("returns all experiments when all are allowed", func() {
			input := []string{"managed-services", "custom-service-image"}
			result := gcp.FilterExperiments(input, input)
			Expect(result).To(ConsistOf("managed-services", "custom-service-image"))
		})

		It("returns empty slice when no experiments are allowed", func() {
			input := []string{"secret-management", "sub-path-mount"}
			result := gcp.FilterExperiments(input, []string{})
			Expect(result).To(BeEmpty())
		})

		It("handles empty input", func() {
			result := gcp.FilterExperiments([]string{}, []string{"secret-management"})
			Expect(result).To(BeEmpty())
		})
	})

	Describe("ApplyLTSCompat", func() {
		var spec *gcp.LTSSpec

		BeforeEach(func() {
			spec = gcp.FindLTSSpec("codesphere-lts-v1.77.2")
			Expect(spec).NotTo(BeNil())
		})

		It("keeps only supported experiments", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{"managed-services", "custom-service-image", "secret-management", "ms-in-ls", "sub-path-mount"},
			}
			gcp.ApplyLTSCompat(cfg, spec)
			Expect(cfg.Experiments).To(ConsistOf("managed-services", "custom-service-image", "ms-in-ls"))
			Expect(cfg.Experiments).NotTo(ContainElement("secret-management"))
			Expect(cfg.Experiments).NotTo(ContainElement("sub-path-mount"))
		})

		It("clears managed services (LTS 1.77.2 schema requires full provider definitions)", func() {
			cfg := &files.CodesphereConfig{
				ManagedServices: []files.ManagedServiceConfig{
					{
						Name:        "postgres",
						Version:     "v1",
						Author:      "Codesphere",
						DisplayName: "PostgreSQL",
						Description: "Open-source database",
						Category:    "Database",
						Scope:       "global",
						Backend: files.ManagedServiceBackend{
							API: files.ManagedServiceAPI{
								Endpoint: "http://ms-backend-postgres.postgres-operator:3000/api/v1/postgres",
							},
						},
						Plans: []files.ServicePlan{{ID: 0, Name: "Small"}},
						Capabilities: &files.ManagedServiceCapabilities{
							Pause:   true,
							Backups: true,
						},
					},
					{
						Name:    "s3",
						Version: "v1",
						Backend: files.ManagedServiceBackend{
							API: files.ManagedServiceAPI{
								Endpoint: "http://localhost:8080",
							},
						},
					},
				},
			}

			gcp.ApplyLTSCompat(cfg, spec)

			Expect(cfg.ManagedServices).To(BeNil())
		})

		It("handles nil managed services slice", func() {
			cfg := &files.CodesphereConfig{
				ManagedServices: nil,
				Experiments:     []string{"custom-service-image"},
			}
			Expect(func() { gcp.ApplyLTSCompat(cfg, spec) }).NotTo(Panic())
			Expect(cfg.ManagedServices).To(BeEmpty())
		})

		It("handles empty experiments slice", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{},
			}
			gcp.ApplyLTSCompat(cfg, spec)
			Expect(cfg.Experiments).To(BeEmpty())
		})
	})

	Describe("GenerateLTSJumpboxFiles", func() {
		var (
			root *files.RootConfig
			spec *gcp.LTSSpec
		)

		BeforeEach(func() {
			spec = gcp.FindLTSSpec("codesphere-lts-v1.77.2")
			Expect(spec).NotTo(BeNil())

			root = &files.RootConfig{
				Codesphere: files.CodesphereConfig{
					Experiments: []string{"managed-services", "custom-service-image", "secret-management", "ms-in-ls", "sub-path-mount"},
					ManagedServices: []files.ManagedServiceConfig{
						{Name: "postgres", Version: "v1", Author: "Codesphere"},
						{Name: "s3", Version: "v1"},
					},
				},
			}
		})

		It("does not modify the original root config", func() {
			_, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(root.Codesphere.Experiments).To(ContainElement("secret-management"))
			Expect(root.Codesphere.ManagedServices[0].Author).To(Equal("Codesphere"))
			Expect(root.CodesphereConfigPath).To(BeEmpty())
		})

		It("clears GeneratedForVersion in the LTS config so the old installer ignores it", func() {
			root.GeneratedForVersion = "codesphere-lts-v1.77.2"
			jumpboxBytes, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("generatedForVersion"))
			// Original must not be modified
			Expect(root.GeneratedForVersion).To(Equal("codesphere-lts-v1.77.2"))
		})

		It("returns codesphere bytes with compat applied", func() {
			jumpboxBytes, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(jumpboxBytes).NotTo(BeEmpty())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("managedServices"))
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("secret-management"))
		})

		It("returns jumpbox config bytes with inline compat-stripped codesphere", func() {
			jumpboxBytes, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(jumpboxBytes).NotTo(BeEmpty())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("/etc/codesphere/codesphere.yaml"))
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("managedServices"))
		})

		It("jumpbox config bytes do not contain unsupported experiment", func() {
			jumpboxBytes, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("secret-management"))
		})
	})

	Describe("LTSConfigFileSuffix", func() {
		It("converts the LTS 1.77.2 version string to a filesystem-safe suffix", func() {
			Expect(gcp.LTSConfigFileSuffix("codesphere-lts-v1.77.2")).To(Equal("lts-1_77_2"))
		})
	})

	Describe("LocalLTSConfigPath", func() {
		It("returns config-lts-1_77_2.yaml in same directory as config.yaml", func() {
			spec := gcp.FindLTSSpec("codesphere-lts-v1.77.2")
			Expect(gcp.LocalLTSConfigPath("config.yaml", spec)).To(Equal("config-lts-1_77_2.yaml"))
		})

		It("uses the directory of the given config path", func() {
			spec := gcp.FindLTSSpec("codesphere-lts-v1.77.2")
			Expect(gcp.LocalLTSConfigPath("/etc/codesphere/config.yaml", spec)).To(Equal("/etc/codesphere/config-lts-1_77_2.yaml"))
		})
	})
})
