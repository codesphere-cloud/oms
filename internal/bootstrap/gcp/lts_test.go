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
			Expect(spec.ClearManagedServices).To(BeFalse())
			Expect(spec.RequiresCephMasterWatcher).To(BeTrue())
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

	Describe("ValidateExperiments", func() {
		It("returns nil when all experiments are allowed", func() {
			err := gcp.ValidateExperiments(
				[]string{"managed-services", "custom-service-image"},
				[]string{"managed-services", "custom-service-image", "ms-in-ls"},
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error for unsupported experiments", func() {
			err := gcp.ValidateExperiments(
				[]string{"managed-services", "secret-management", "sub-path-mount"},
				[]string{"managed-services", "custom-service-image", "ms-in-ls"},
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported experiments"))
			Expect(err.Error()).To(ContainSubstring("secret-management"))
			Expect(err.Error()).To(ContainSubstring("sub-path-mount"))
		})

		It("returns nil for empty experiments", func() {
			Expect(gcp.ValidateExperiments([]string{}, []string{"managed-services"})).NotTo(HaveOccurred())
		})

		It("returns error when all experiments are unsupported", func() {
			err := gcp.ValidateExperiments(
				[]string{"secret-management", "sub-path-mount"},
				[]string{"managed-services"},
			)
			Expect(err).To(HaveOccurred())
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
			err := gcp.ApplyLTSCompat(cfg, spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported experiments"))
			Expect(err.Error()).To(ContainSubstring("secret-management"))
			Expect(err.Error()).To(ContainSubstring("sub-path-mount"))
		})

		It("succeeds when all experiments are supported", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{"managed-services", "custom-service-image", "ms-in-ls"},
			}
			err := gcp.ApplyLTSCompat(cfg, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Experiments).To(ConsistOf("managed-services", "custom-service-image", "ms-in-ls"))
		})

		It("preserves managed services (LTS 1.77.2 keeps them in separate codesphere config)", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{"managed-services"},
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

			err := gcp.ApplyLTSCompat(cfg, spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.ManagedServices).To(HaveLen(2))
			Expect(cfg.ManagedServices[0].Name).To(Equal("postgres"))
			Expect(cfg.ManagedServices[0].Author).To(Equal("Codesphere"))
		})

		It("handles nil managed services slice", func() {
			cfg := &files.CodesphereConfig{
				ManagedServices: nil,
				Experiments:     []string{"custom-service-image"},
			}
			err := gcp.ApplyLTSCompat(cfg, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ManagedServices).To(BeEmpty())
		})

		It("handles empty experiments slice", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{},
			}
			err := gcp.ApplyLTSCompat(cfg, spec)
			Expect(err).NotTo(HaveOccurred())
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
					Experiments: []string{"managed-services", "custom-service-image", "ms-in-ls"},
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
			Expect(root.Codesphere.Experiments).To(ConsistOf("managed-services", "custom-service-image", "ms-in-ls"))
			Expect(root.Codesphere.ManagedServices[0].Author).To(Equal("Codesphere"))
			Expect(root.CodesphereConfigPath).To(BeEmpty())
		})

		It("clears GeneratedForVersion in the LTS config so the old installer ignores it", func() {
			root.GeneratedForVersion = "codesphere-lts-v1.77.2"
			jumpboxBytes, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("generatedForVersion"))
			Expect(root.GeneratedForVersion).To(Equal("codesphere-lts-v1.77.2"))
		})

		It("omits the codesphere key entirely from the LTS config", func() {
			jumpboxBytes, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(jumpboxBytes).NotTo(BeEmpty())
			// LTS 1.77.2 installer rejects both inline objects and string-path refs;
			// only the absent key is accepted.
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("codesphere:"))
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("managedServices"))
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("managed-services"))
		})

		It("errors when unsupported experiments are in the root config", func() {
			root.Codesphere.Experiments = []string{"managed-services", "unsupported-exp"}
			_, err := gcp.GenerateLTSJumpboxFiles(root, spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported experiments"))
			Expect(err.Error()).To(ContainSubstring("unsupported-exp"))
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
