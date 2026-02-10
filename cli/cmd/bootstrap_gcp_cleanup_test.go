// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/util"
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

	Describe("CleanupDeps structure", func() {
		Context("when created", func() {
			It("should hold all required dependencies", func() {
				mockGCPClient := gcp.NewMockGCPClientManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())
				stlog := bootstrap.NewStepLogger(false)
				confirmReader := bytes.NewBufferString("test-project\n")

				deps := &cmd.CleanupDeps{
					GCPClient:     mockGCPClient,
					FileIO:        mockFileIO,
					StepLogger:    stlog,
					ConfirmReader: confirmReader,
					InfraFilePath: "/tmp/test-infra.json",
				}

				Expect(deps.GCPClient).ToNot(BeNil())
				Expect(deps.FileIO).ToNot(BeNil())
				Expect(deps.StepLogger).ToNot(BeNil())
				Expect(deps.ConfirmReader).ToNot(BeNil())
				Expect(deps.InfraFilePath).To(Equal("/tmp/test-infra.json"))
			})
		})
	})

	Describe("executeCleanup", func() {
		var (
			cleanupCmd    *cmd.BootstrapGcpCleanupCmd
			mockGCPClient *gcp.MockGCPClientManager
			mockFileIO    *util.MockFileIO
			deps          *cmd.CleanupDeps
		)

		BeforeEach(func() {
			mockGCPClient = gcp.NewMockGCPClientManager(GinkgoT())
			mockFileIO = util.NewMockFileIO(GinkgoT())

			cleanupCmd = &cmd.BootstrapGcpCleanupCmd{
				Opts: &cmd.BootstrapGcpCleanupOpts{
					GlobalOptions:  globalOpts,
					ProjectID:      "",
					Force:          false,
					SkipDNSCleanup: false,
				},
			}

			deps = &cmd.CleanupDeps{
				GCPClient:     mockGCPClient,
				FileIO:        mockFileIO,
				StepLogger:    bootstrap.NewStepLogger(false),
				ConfirmReader: bytes.NewBufferString(""),
				InfraFilePath: "/tmp/test-infra.json",
			}
		})

		Context("when no project ID is provided and infra file doesn't exist", func() {
			It("should return an error", func() {
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no project ID provided and no infra file found"))
			})
		})

		Context("when infra file exists but has empty project ID", func() {
			It("should return an error about empty project ID", func() {
				emptyEnv := gcp.CodesphereEnvironment{
					ProjectID: "", // Empty project ID
				}
				envData, _ := json.Marshal(emptyEnv)

				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(true)
				mockFileIO.EXPECT().ReadFile("/tmp/test-infra.json").Return(envData, nil)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains empty project ID"))
			})
		})

		Context("when infra file exists with valid project ID", func() {
			It("should load project ID from infra file and verify OMS management", func() {
				validEnv := gcp.CodesphereEnvironment{
					ProjectID: "test-project-123",
				}
				envData, _ := json.Marshal(validEnv)

				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(true)
				mockFileIO.EXPECT().ReadFile("/tmp/test-infra.json").Return(envData, nil)
				mockGCPClient.EXPECT().IsOMSManagedProject("test-project-123").Return(false, nil)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("was not bootstrapped by OMS"))
			})
		})

		Context("when project ID is provided via flag", func() {
			It("should use the provided project ID", func() {
				cleanupCmd.Opts.ProjectID = "flag-project"
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)
				mockGCPClient.EXPECT().IsOMSManagedProject("flag-project").Return(false, nil)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("flag-project was not bootstrapped by OMS"))
			})
		})

		Context("when OMS management check fails", func() {
			It("should return the verification error", func() {
				cleanupCmd.Opts.ProjectID = "test-project"
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)
				mockGCPClient.EXPECT().IsOMSManagedProject("test-project").Return(false, errors.New("API error"))

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to verify project"))
			})
		})

		Context("when force flag is set", func() {
			It("should skip OMS management check and proceed to confirmation", func() {
				cleanupCmd.Opts.ProjectID = "test-project"
				cleanupCmd.Opts.Force = true
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)
				mockGCPClient.EXPECT().DeleteProject("test-project").Return(nil)
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when confirmation does not match", func() {
			It("should abort the cleanup", func() {
				cleanupCmd.Opts.ProjectID = "test-project"
				deps.ConfirmReader = bytes.NewBufferString("wrong-input\n")
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)
				mockGCPClient.EXPECT().IsOMSManagedProject("test-project").Return(true, nil)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("confirmation did not match project ID"))
			})
		})

		Context("when infra file read fails", func() {
			It("should return the read error", func() {
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(true)
				mockFileIO.EXPECT().ReadFile("/tmp/test-infra.json").Return(nil, os.ErrPermission)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read gcp infra file"))
			})
		})

		Context("when infra file contains invalid JSON", func() {
			It("should return the unmarshal error", func() {
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(true)
				mockFileIO.EXPECT().ReadFile("/tmp/test-infra.json").Return([]byte("invalid-json"), nil)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal gcp infra file"))
			})
		})

		Context("when DNS cleanup is enabled and infra has DNS info", func() {
			It("should attempt DNS cleanup before deleting project", func() {
				cleanupCmd.Opts.ProjectID = "test-project"
				cleanupCmd.Opts.Force = true

				validEnv := gcp.CodesphereEnvironment{
					ProjectID:   "test-project",
					BaseDomain:  "example.com",
					DNSZoneName: "test-zone",
				}
				envData, _ := json.Marshal(validEnv)

				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(true)
				mockFileIO.EXPECT().ReadFile("/tmp/test-infra.json").Return(envData, nil)
				mockGCPClient.EXPECT().DeleteDNSRecordSets("test-project", "test-zone", "example.com").Return(nil)
				mockGCPClient.EXPECT().DeleteProject("test-project").Return(nil)
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when skip-dns-cleanup flag is set", func() {
			It("should skip DNS cleanup even when infra has DNS info", func() {
				cleanupCmd.Opts.ProjectID = "test-project"
				cleanupCmd.Opts.Force = true
				cleanupCmd.Opts.SkipDNSCleanup = true

				validEnv := gcp.CodesphereEnvironment{
					ProjectID:   "test-project",
					BaseDomain:  "example.com",
					DNSZoneName: "test-zone",
				}
				envData, _ := json.Marshal(validEnv)

				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(true)
				mockFileIO.EXPECT().ReadFile("/tmp/test-infra.json").Return(envData, nil)
				// DNS deletion should NOT be called
				mockGCPClient.EXPECT().DeleteProject("test-project").Return(nil)
				mockFileIO.EXPECT().Exists("/tmp/test-infra.json").Return(false)

				err := cleanupCmd.ExecuteCleanup(deps)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
