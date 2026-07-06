// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"context"
	"os"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
)

var _ = Describe("InstallCodesphereCmd", func() {
	var (
		c          cmd.InstallCodesphereCmd
		opts       *cmd.InstallCodesphereOpts
		globalOpts *cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.InstallCodesphereOpts{
			GlobalOptions: globalOpts,
			Package:       "codesphere-v1.66.0-installer-lite.tar.gz",
			Force:         false,
		}
		c = cmd.InstallCodesphereCmd{
			Opts: opts,
			Env:  mockEnv,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("calls GetOmsWorkdir and fails on non-linux platform", func() {
			c.Opts.Package = "test-package.tar.gz"

			tempConfigFile, err := os.CreateTemp("", "test-config.yaml")
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempConfigFile.Name()) }()

			_, err = tempConfigFile.WriteString("codesphere:\n  deployConfig:\n    images: {}\n")
			Expect(err).To(BeNil())
			_ = tempConfigFile.Close()

			c.Opts.Configs = []string{tempConfigFile.Name()}
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			runCmd := &cobra.Command{}
			runCmd.SetContext(context.Background())
			err = c.RunE(runCmd, []string{})

			Expect(err).To(HaveOccurred())
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux platform
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on package extraction since the package doesn't exist
				Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
			}
		})
	})

})

var _ = Describe("AddInstallCodesphereCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "install"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the codesphere command with correct properties and flags", func() {
		cmd.AddInstallCodesphereCmd(parentCmd, globalOpts)

		var codesphereCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "codesphere" {
				codesphereCmd = c
				break
			}
		}

		Expect(codesphereCmd).NotTo(BeNil())
		Expect(codesphereCmd.Use).To(Equal("codesphere"))
		Expect(codesphereCmd.Short).To(Equal("Install a Codesphere instance"))
		Expect(codesphereCmd.Long).To(ContainSubstring("Uses the private-cloud-installer.js script included in the package to perform the installation."))
		Expect(codesphereCmd.RunE).NotTo(BeNil())

		// Check flags (registered as PersistentFlags so subcommands inherit them)
		packageFlag := codesphereCmd.PersistentFlags().Lookup("package")
		Expect(packageFlag).NotTo(BeNil())
		Expect(packageFlag.Shorthand).To(Equal("p"))

		forceFlag := codesphereCmd.PersistentFlags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.DefValue).To(Equal("false"))

		configFlag := codesphereCmd.PersistentFlags().Lookup("config")
		Expect(configFlag).NotTo(BeNil())
		Expect(configFlag.Shorthand).To(Equal("c"))

		privKeyFlag := codesphereCmd.PersistentFlags().Lookup("priv-key")
		Expect(privKeyFlag).NotTo(BeNil())
		Expect(privKeyFlag.Shorthand).To(Equal("k"))

		vaultFlag := codesphereCmd.PersistentFlags().Lookup("vault")
		Expect(vaultFlag).NotTo(BeNil())
		Expect(vaultFlag.DefValue).To(Equal(""))

		skipStepFlag := codesphereCmd.PersistentFlags().Lookup("skip-steps")
		Expect(skipStepFlag).NotTo(BeNil())
		Expect(skipStepFlag.Shorthand).To(Equal("s"))
	})
})
