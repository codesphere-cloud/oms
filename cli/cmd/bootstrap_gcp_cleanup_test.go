// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
)

var _ = Describe("BootstrapGcpCleanupCmd", func() {
	var (
		opts       *cmd.BootstrapGcpCleanupOpts
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.BootstrapGcpCleanupOpts{
			GlobalOptions:  globalOpts,
			ProjectID:      "",
			Force:          false,
			SkipDNSCleanup: false,
		}
	})

	Describe("BootstrapGcpCleanupOpts structure", func() {
		Context("when initialized", func() {
			It("should have correct default values", func() {
				Expect(opts.ProjectID).To(Equal(""))
				Expect(opts.Force).To(BeFalse())
				Expect(opts.SkipDNSCleanup).To(BeFalse())
				Expect(opts.GlobalOptions).ToNot(BeNil())
			})
		})

		Context("when flags are set", func() {
			It("should store flag values correctly", func() {
				opts.ProjectID = "test-project-123"
				opts.Force = true
				opts.SkipDNSCleanup = true

				Expect(opts.ProjectID).To(Equal("test-project-123"))
				Expect(opts.Force).To(BeTrue())
				Expect(opts.SkipDNSCleanup).To(BeTrue())
			})
		})
	})

	Describe("CodesphereEnvironment JSON marshaling", func() {
		Context("when environment is complete", func() {
			It("should marshal and unmarshal correctly", func() {
				env := gcp.CodesphereEnvironment{
					ProjectID:    "test-project",
					BaseDomain:   "example.com",
					DNSZoneName:  "test-zone",
					DNSProjectID: "dns-project",
				}

				data, err := json.Marshal(env)
				Expect(err).NotTo(HaveOccurred())

				var decoded gcp.CodesphereEnvironment
				err = json.Unmarshal(data, &decoded)
				Expect(err).NotTo(HaveOccurred())

				Expect(decoded.ProjectID).To(Equal("test-project"))
				Expect(decoded.BaseDomain).To(Equal("example.com"))
				Expect(decoded.DNSZoneName).To(Equal("test-zone"))
				Expect(decoded.DNSProjectID).To(Equal("dns-project"))
			})
		})

		Context("when environment is minimal", func() {
			It("should handle missing DNS fields", func() {
				env := gcp.CodesphereEnvironment{
					ProjectID: "test-project",
				}

				data, err := json.Marshal(env)
				Expect(err).NotTo(HaveOccurred())

				var decoded gcp.CodesphereEnvironment
				err = json.Unmarshal(data, &decoded)
				Expect(err).NotTo(HaveOccurred())

				Expect(decoded.ProjectID).To(Equal("test-project"))
				Expect(decoded.BaseDomain).To(Equal(""))
				Expect(decoded.DNSZoneName).To(Equal(""))
			})
		})
	})

	Describe("AddBootstrapGcpCleanupCmd", func() {
		Context("when adding command", func() {
			It("should not panic when adding to parent command", func() {
				Expect(func() {
					parentCmd := &cobra.Command{
						Use: "bootstrap-gcp",
					}
					cmd.AddBootstrapGcpCleanupCmd(parentCmd, globalOpts)
				}).NotTo(Panic())
			})

			It("should create command with correct flags", func() {
				parentCmd := &cobra.Command{
					Use: "bootstrap-gcp",
				}
				cmd.AddBootstrapGcpCleanupCmd(parentCmd, globalOpts)

				// Verify the cleanup subcommand was added
				cleanupCmd, _, err := parentCmd.Find([]string{"cleanup"})
				Expect(err).NotTo(HaveOccurred())
				Expect(cleanupCmd).NotTo(BeNil())
				Expect(cleanupCmd.Use).To(Equal("cleanup"))

				// Verify flags exist
				projectIDFlag := cleanupCmd.Flags().Lookup("project-id")
				Expect(projectIDFlag).NotTo(BeNil())

				forceFlag := cleanupCmd.Flags().Lookup("force")
				Expect(forceFlag).NotTo(BeNil())

				skipDNSFlag := cleanupCmd.Flags().Lookup("skip-dns-cleanup")
				Expect(skipDNSFlag).NotTo(BeNil())
			})
		})
	})
})
