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
	var (
		globalOpts *cmd.GlobalOptions
		parentCmd  *cobra.Command
	)

	BeforeEach(func() {
		globalOpts = &cmd.GlobalOptions{}
		parentCmd = &cobra.Command{Use: "bootstrap-gcp"}
		cmd.AddBootstrapGcpRestartVMsCmd(parentCmd, globalOpts)
	})

	findRestartCmd := func() *cobra.Command {
		c, _, err := parentCmd.Find([]string{"restart-vms"})
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
		return c
	}

	Describe("command registration", func() {
		It("registers restart-vms with the expected flags", func() {
			restartCmd := findRestartCmd()
			Expect(restartCmd.Use).To(Equal("restart-vms"))

			for _, flag := range []string{"project-id", "zone", "name"} {
				Expect(restartCmd.Flags().Lookup(flag)).NotTo(BeNil(), "expected flag %q to exist", flag)
			}
		})

		It("binds flag values to opts", func() {
			restartCmd := findRestartCmd()

			Expect(restartCmd.Flags().Set("project-id", "my-proj")).To(Succeed())
			Expect(restartCmd.Flags().Set("zone", "eu-west1-b")).To(Succeed())
			Expect(restartCmd.Flags().Set("name", "jumpbox")).To(Succeed())

			val, err := restartCmd.Flags().GetString("project-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("my-proj"))

			val, err = restartCmd.Flags().GetString("zone")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("eu-west1-b"))

			val, err = restartCmd.Flags().GetString("name")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("jumpbox"))
		})
	})

	Describe("flag validation", func() {
		It("rejects --project-id without --zone", func() {
			restartCmd := findRestartCmd()
			Expect(restartCmd.Flags().Set("project-id", "my-proj")).To(Succeed())

			err := restartCmd.RunE(restartCmd, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--project-id and --zone must be provided together"))
		})

		It("rejects --zone without --project-id", func() {
			restartCmd := findRestartCmd()
			Expect(restartCmd.Flags().Set("zone", "us-central1-a")).To(Succeed())

			err := restartCmd.RunE(restartCmd, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--project-id and --zone must be provided together"))
		})
	})
})
