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
	Describe("IsLTS177", func() {
		It("returns true for the exact LTS 1.77.2 version string", func() {
			Expect(gcp.IsLTS177("codesphere-lts-v1.77.2")).To(BeTrue())
		})

		It("returns false for another LTS version", func() {
			Expect(gcp.IsLTS177("codesphere-lts-v1.80.0")).To(BeFalse())
		})

		It("returns false for an empty string", func() {
			Expect(gcp.IsLTS177("")).To(BeFalse())
		})

		It("returns false for a non-LTS version", func() {
			Expect(gcp.IsLTS177("master")).To(BeFalse())
		})

		It("returns false for a partial match", func() {
			Expect(gcp.IsLTS177("codesphere-lts-v1.77.2-extra")).To(BeFalse())
		})
	})

	Describe("FilterExperiments", func() {
		It("removes unsupported experiments", func() {
			input := []string{"managed-services", "custom-service-image", "secret-management", "ms-in-ls", "sub-path-mount"}
			unsupported := []string{"secret-management", "sub-path-mount"}
			result := gcp.FilterExperiments(input, unsupported)
			Expect(result).To(ConsistOf("managed-services", "custom-service-image", "ms-in-ls"))
		})

		It("returns the same slice when nothing is unsupported", func() {
			input := []string{"managed-services", "custom-service-image"}
			result := gcp.FilterExperiments(input, []string{})
			Expect(result).To(ConsistOf("managed-services", "custom-service-image"))
		})

		It("returns empty slice when all experiments are unsupported", func() {
			input := []string{"secret-management", "sub-path-mount"}
			unsupported := []string{"secret-management", "sub-path-mount"}
			result := gcp.FilterExperiments(input, unsupported)
			Expect(result).To(BeEmpty())
		})

		It("handles empty input", func() {
			result := gcp.FilterExperiments([]string{}, []string{"secret-management"})
			Expect(result).To(BeEmpty())
		})
	})

	Describe("ApplyLTS177Compat", func() {
		It("removes unsupported experiments from the config", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{"managed-services", "custom-service-image", "secret-management", "ms-in-ls", "sub-path-mount"},
			}
			gcp.ApplyLTS177Compat(cfg)
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

			gcp.ApplyLTS177Compat(cfg)

			Expect(cfg.ManagedServices).To(BeNil())
		})

		It("handles nil managed services slice", func() {
			cfg := &files.CodesphereConfig{
				ManagedServices: nil,
				Experiments:     []string{"custom-service-image"},
			}
			Expect(func() { gcp.ApplyLTS177Compat(cfg) }).NotTo(Panic())
			Expect(cfg.ManagedServices).To(BeEmpty())
		})

		It("handles empty experiments slice", func() {
			cfg := &files.CodesphereConfig{
				Experiments: []string{},
			}
			gcp.ApplyLTS177Compat(cfg)
			Expect(cfg.Experiments).To(BeEmpty())
		})
	})

	Describe("GenerateLTS177JumpboxFiles", func() {
		var root *files.RootConfig

		BeforeEach(func() {
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
			_, _, err := gcp.GenerateLTS177JumpboxFiles(root)
			Expect(err).NotTo(HaveOccurred())
			Expect(root.Codesphere.Experiments).To(ContainElement("secret-management"))
			Expect(root.Codesphere.ManagedServices[0].Author).To(Equal("Codesphere"))
			Expect(root.CodesphereConfigPath).To(BeEmpty())
		})

		It("returns codesphere bytes with compat applied", func() {
			_, csBytes, err := gcp.GenerateLTS177JumpboxFiles(root)
			Expect(err).NotTo(HaveOccurred())
			Expect(csBytes).NotTo(BeEmpty())
			csYAML := string(csBytes)
			// ManagedServices is cleared in LTS 1.77.2 compat mode
			Expect(csYAML).NotTo(ContainSubstring("managedServices"))
			Expect(csYAML).NotTo(ContainSubstring("secret-management"))
		})

		It("returns jumpbox config bytes with inline compat-stripped codesphere", func() {
			jumpboxBytes, _, err := gcp.GenerateLTS177JumpboxFiles(root)
			Expect(err).NotTo(HaveOccurred())
			Expect(jumpboxBytes).NotTo(BeEmpty())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("/etc/codesphere/codesphere.yaml"))
			// ManagedServices is cleared in LTS 1.77.2 compat mode
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("managedServices"))
		})

		It("jumpbox config bytes do not contain inline codesphere fields", func() {
			jumpboxBytes, _, err := gcp.GenerateLTS177JumpboxFiles(root)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(jumpboxBytes)).NotTo(ContainSubstring("secret-management"))
		})
	})

	Describe("LTS177LocalJumpboxConfigPath", func() {
		It("returns config-jumpbox.yaml in same directory as config.yaml", func() {
			Expect(gcp.LTS177LocalJumpboxConfigPath("config.yaml")).To(Equal("config-jumpbox.yaml"))
		})

		It("uses the directory of the given config path", func() {
			Expect(gcp.LTS177LocalJumpboxConfigPath("/etc/codesphere/config.yaml")).To(Equal("/etc/codesphere/config-jumpbox.yaml"))
		})
	})

	Describe("LTS177LocalCodesphereConfigPath", func() {
		It("returns codesphere.yaml in same directory as config.yaml", func() {
			Expect(gcp.LTS177LocalCodesphereConfigPath("config.yaml")).To(Equal("codesphere.yaml"))
		})

		It("uses the directory of the given config path", func() {
			Expect(gcp.LTS177LocalCodesphereConfigPath("/etc/codesphere/config.yaml")).To(Equal("/etc/codesphere/codesphere.yaml"))
		})
	})
})
