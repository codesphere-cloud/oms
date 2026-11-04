// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"testing"

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
