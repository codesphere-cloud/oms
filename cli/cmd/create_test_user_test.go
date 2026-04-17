// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
)

var _ = Describe("CreateTestUser", func() {
	Context("AddCreateTestUserCmd", func() {
		var createCmd cobra.Command
		var opts *cmd.GlobalOptions

		BeforeEach(func() {
			createCmd = cobra.Command{}
			opts = &cmd.GlobalOptions{}
		})

		It("accepts valid flags with all required flags set", func() {
			createCmd.SetArgs([]string{
				"test-user",
				"--postgres-host", "localhost",
				"--postgres-password", "secret",
			})

			cmd.AddCreateTestUserCmd(&createCmd, opts)

			createCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := createCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when --postgres-host is missing", func() {
			createCmd.SetArgs([]string{
				"test-user",
				"--postgres-password", "secret",
			})

			cmd.AddCreateTestUserCmd(&createCmd, opts)

			createCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := createCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("postgres-host"))
		})

		It("fails when --postgres-password is missing", func() {
			createCmd.SetArgs([]string{
				"test-user",
				"--postgres-host", "localhost",
			})

			cmd.AddCreateTestUserCmd(&createCmd, opts)

			createCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := createCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("postgres-password"))
		})

		It("accepts optional flags with custom values", func() {
			createCmd.SetArgs([]string{
				"test-user",
				"--postgres-host", "db.example.com",
				"--postgres-password", "secret",
				"--postgres-port", "5433",
				"--postgres-user", "admin",
				"--postgres-db", "mydb",
				"--ssl-mode", "require",
			})

			cmd.AddCreateTestUserCmd(&createCmd, opts)

			createCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := createCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
