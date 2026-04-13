// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
)

var _ = Describe("AddCmd", func() {
	It("inherits the parent Args validator when the child does not define one", func() {
		parent := &cobra.Command{
			Use:           "root",
			Args:          cobra.NoArgs,
			SilenceErrors: true,
			SilenceUsage:  true,
		}
		child := &cobra.Command{
			Use:  "child",
			RunE: func(_ *cobra.Command, _ []string) error { return nil },
		}
		cmd.AddCmd(parent, child)

		parent.SetArgs([]string{"child", "extra"})
		err := parent.Execute()

		Expect(err).To(HaveOccurred())
		Expect(parent.Commands()).To(ContainElement(child))
	})

	It("keeps a child-specific Args validator when one is explicitly set", func() {
		parent := &cobra.Command{
			Use:           "root",
			Args:          cobra.NoArgs,
			SilenceErrors: true,
			SilenceUsage:  true,
		}

		capturedArgs := []string{}
		child := &cobra.Command{
			Use:  "child",
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				capturedArgs = args
				return nil
			},
		}
		cmd.AddCmd(parent, child)

		parent.SetArgs([]string{"child", "value"})
		err := parent.Execute()

		Expect(err).NotTo(HaveOccurred())
		Expect(capturedArgs).To(Equal([]string{"value"}))
		Expect(parent.Commands()).To(ContainElement(child))
	})
})
