// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

var _ = Describe("command usage on errors", func() {
	execute := func(root *cobra.Command, args ...string) (string, error) {
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(args)
		err := root.Execute()

		return output.String(), err
	}

	newCommandTree := func() (*cobra.Command, *cobra.Command) {
		root := &cobra.Command{Use: "oms", Args: cobra.NoArgs}
		child := &cobra.Command{
			Use:  "install",
			Args: cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				return errors.New("installation failed")
			},
		}
		child.Flags().String("config", "", "installation config")
		root.AddCommand(child)
		silenceUsageOnRunErrors(root)

		return root, child
	}

	It("does not print usage for an operational RunE error", func() {
		root, _ := newCommandTree()

		output, err := execute(root, "install")

		Expect(err).To(MatchError("installation failed"))
		Expect(output).To(ContainSubstring("Error: installation failed"))
		Expect(output).NotTo(ContainSubstring("Usage:"))
	})

	DescribeTable("prints usage for invalid command input",
		func(configure func(*cobra.Command), args []string) {
			root, child := newCommandTree()
			configure(child)

			output, err := execute(root, args...)

			Expect(err).To(HaveOccurred())
			Expect(output).To(ContainSubstring("Usage:"))
		},
		Entry("unexpected positional arguments", func(*cobra.Command) {}, []string{"install", "unexpected"}),
		Entry("unknown flags", func(*cobra.Command) {}, []string{"install", "--unknown"}),
		Entry("missing required flags", func(command *cobra.Command) {
			Expect(command.MarkFlagRequired("config")).To(Succeed())
		}, []string{"install"}),
	)
})
