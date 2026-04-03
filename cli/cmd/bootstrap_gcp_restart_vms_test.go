// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
)

var _ = Describe("BootstrapGcpRestartVMsCmd", func() {
	var globalOpts *cmd.GlobalOptions

	BeforeEach(func() {
		globalOpts = &cmd.GlobalOptions{}
	})

	Describe("BootstrapGcpRestartVMsOpts structure", func() {
		Context("when initialized", func() {
			It("should have correct default values", func() {
				opts := &cmd.BootstrapGcpRestartVMsOpts{
					GlobalOptions: globalOpts,
				}
				Expect(opts.ProjectID).To(Equal(""))
				Expect(opts.Zone).To(Equal(""))
				Expect(opts.Name).To(Equal(""))
			})

			It("should store provided values", func() {
				opts := &cmd.BootstrapGcpRestartVMsOpts{
					GlobalOptions: globalOpts,
					ProjectID:     "my-project",
					Zone:          "us-central1-a",
					Name:          "jumpbox",
				}
				Expect(opts.ProjectID).To(Equal("my-project"))
				Expect(opts.Zone).To(Equal("us-central1-a"))
				Expect(opts.Name).To(Equal("jumpbox"))
			})
		})
	})

	Describe("AddBootstrapGcpRestartVMsCmd", func() {
		Context("when adding command", func() {
			It("should not panic when adding to parent command", func() {
				Expect(func() {
					parentCmd := &cobra.Command{
						Use: "bootstrap-gcp",
					}
					cmd.AddBootstrapGcpRestartVMsCmd(parentCmd, globalOpts)
				}).NotTo(Panic())
			})

			It("should create command with correct flags", func() {
				parentCmd := &cobra.Command{
					Use: "bootstrap-gcp",
				}
				cmd.AddBootstrapGcpRestartVMsCmd(parentCmd, globalOpts)

				restartCmd, _, err := parentCmd.Find([]string{"restart-vms"})
				Expect(err).NotTo(HaveOccurred())
				Expect(restartCmd).NotTo(BeNil())
				Expect(restartCmd.Use).To(Equal("restart-vms"))

				projectIDFlag := restartCmd.Flags().Lookup("project-id")
				Expect(projectIDFlag).NotTo(BeNil())

				zoneFlag := restartCmd.Flags().Lookup("zone")
				Expect(zoneFlag).NotTo(BeNil())

				nameFlag := restartCmd.Flags().Lookup("name")
				Expect(nameFlag).NotTo(BeNil())
			})

			It("should bind flag values to opts", func() {
				parentCmd := &cobra.Command{
					Use: "bootstrap-gcp",
				}
				cmd.AddBootstrapGcpRestartVMsCmd(parentCmd, globalOpts)

				restartCmd, _, err := parentCmd.Find([]string{"restart-vms"})
				Expect(err).NotTo(HaveOccurred())
				Expect(restartCmd).NotTo(BeNil())

				err = restartCmd.Flags().Set("project-id", "flag-project")
				Expect(err).NotTo(HaveOccurred())
				projectIDVal, err := restartCmd.Flags().GetString("project-id")
				Expect(err).NotTo(HaveOccurred())
				Expect(projectIDVal).To(Equal("flag-project"))

				err = restartCmd.Flags().Set("zone", "flag-zone")
				Expect(err).NotTo(HaveOccurred())
				zoneVal, err := restartCmd.Flags().GetString("zone")
				Expect(err).NotTo(HaveOccurred())
				Expect(zoneVal).To(Equal("flag-zone"))

				err = restartCmd.Flags().Set("name", "jumpbox")
				Expect(err).NotTo(HaveOccurred())
				nameVal, err := restartCmd.Flags().GetString("name")
				Expect(err).NotTo(HaveOccurred())
				Expect(nameVal).To(Equal("jumpbox"))
			})
		})
	})
})
