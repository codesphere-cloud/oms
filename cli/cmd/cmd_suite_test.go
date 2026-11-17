// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cmd Suite")
}

var _ = Describe("RootCmd", func() {
	Describe("GetRootCmd", func() {
		It("returns a valid root command", func() {
			rootCmd := cmd.GetRootCmd()
			Expect(rootCmd).NotTo(BeNil())
			Expect(rootCmd.Use).To(Equal("oms"))
			Expect(rootCmd.Short).To(Equal("Codesphere Operations Management System (OMS)"))
		})
	})
})

// newCommandWithContext returns a bare command whose context is set, as Cobra does when executing.
func newCommandWithContext() *cobra.Command {
	command := &cobra.Command{}
	command.SetContext(context.Background())

	return command
}
