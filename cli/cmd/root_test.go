// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
)

func findCommand(root *cobra.Command, path ...string) *cobra.Command {
	current := root
	for _, p := range path {
		next, _, err := current.Find([]string{p})
		if err != nil || next == nil {
			return nil
		}
		current = next
	}

	return current
}

var _ = Describe("RootCmd", func() {
	It("rejects positional args for commands configured with no positional args", func() {
		rootCmd := cmd.GetRootCmd()
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true

		licensesCmd := findCommand(rootCmd, "licenses")
		Expect(licensesCmd).NotTo(BeNil())

		rootCmd.SetArgs([]string{"licenses", "extra"})
		err := rootCmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown command \"extra\""))
	})

	It("allows positional args for commands explicitly defining them", func() {
		rootCmd := cmd.GetRootCmd()
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true

		downloadPackageCmd := findCommand(rootCmd, "download", "package")
		Expect(downloadPackageCmd).NotTo(BeNil())

		executed := false
		capturedArgs := []string{}
		downloadPackageCmd.RunE = func(_ *cobra.Command, args []string) error {
			executed = true
			capturedArgs = args
			return nil
		}

		rootCmd.SetArgs([]string{"download", "package", "codesphere-v1.55.0"})
		err := rootCmd.Execute()

		Expect(err).NotTo(HaveOccurred())
		Expect(executed).To(BeTrue())
		Expect(capturedArgs).To(Equal([]string{"codesphere-v1.55.0"}))
	})
})
