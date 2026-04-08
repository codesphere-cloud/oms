// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"

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

		updateAPIKeyCmd := findCommand(rootCmd, "update", "api-key")
		Expect(updateAPIKeyCmd).NotTo(BeNil())

		// Avoid running real command logic; argument validation happens before RunE.
		updateAPIKeyCmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }

		rootCmd.SetArgs([]string{"update", "api-key", "--id", "abc123", "--valid-for", "1d", "extra"})
		err := rootCmd.Execute()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("accepts 0 arg(s), received 1"))
	})

	It("allows positional version for download package despite root cobra.NoArgs", func() {
		rootCmd := cmd.GetRootCmd()
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true

		downloadPackageCmd := findCommand(rootCmd, "download", "package")
		Expect(downloadPackageCmd).NotTo(BeNil())

		executed := false
		downloadPackageCmd.RunE = func(_ *cobra.Command, args []string) error {
			executed = true
			if len(args) != 1 {
				return fmt.Errorf("expected one positional arg, got %d", len(args))
			}

			return nil
		}

		rootCmd.SetArgs([]string{"download", "package", "codesphere-v1.55.0"})
		err := rootCmd.Execute()

		Expect(err).NotTo(HaveOccurred())
		Expect(executed).To(BeTrue())
	})
})
