// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("InstallK0sCmd", func() {
	var (
		c              cmd.InstallK0sCmd
		opts           *cmd.InstallK0sOpts
		globalOpts     *cmd.GlobalOptions
		mockEnv        *env.MockEnv
		mockFileWriter *util.MockFileIO
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.InstallK0sOpts{
			GlobalOptions: globalOpts,
			Version:       "",
			Package:       "",
			Config:        "",
			Force:         false,
		}
		c = cmd.InstallK0sCmd{
			Opts:       *opts,
			Env:        mockEnv,
			FileWriter: mockFileWriter,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockFileWriter.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("calls InstallK0s and fails with network error", func() {
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
		})
	})

	Context("InstallK0s method", func() {
		It("fails when package is not specified and k0s download fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Package = "" // No package specified, should download
			mockPackageManager.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/workdir/test-package/deps/kubernetes/files/k0s")
			mockK0sManager.EXPECT().Download("", false, false).Return("", errors.New("download failed"))

			err := c.InstallK0s(mockPackageManager, mockK0sManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
			Expect(err.Error()).To(ContainSubstring("download failed"))
		})

		It("fails when k0s install fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Package = "" // No package specified, should download
			c.Opts.Config = "/path/to/config.yaml"
			c.Opts.Force = true
			mockPackageManager.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/workdir/test-package/deps/kubernetes/files/k0s")
			mockK0sManager.EXPECT().Download("", true, false).Return("/test/workdir/k0s", nil)
			mockK0sManager.EXPECT().Install("/path/to/config.yaml", "/test/workdir/k0s", true).Return(errors.New("install failed"))

			err := c.InstallK0s(mockPackageManager, mockK0sManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
			Expect(err.Error()).To(ContainSubstring("install failed"))
		})

		It("succeeds when package is not specified and k0s download and install work", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Package = "" // No package specified, should download
			c.Opts.Config = ""  // No config, will use single mode
			mockPackageManager.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/workdir/test-package/deps/kubernetes/files/k0s")
			mockK0sManager.EXPECT().Download("", false, false).Return("/test/workdir/k0s", nil)
			mockK0sManager.EXPECT().Install("", "/test/workdir/k0s", false).Return(nil)

			err := c.InstallK0s(mockPackageManager, mockK0sManager)
			Expect(err).ToNot(HaveOccurred())
		})

		It("succeeds when package is specified and k0s install works", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Package = "test-package.tar.gz" // Package specified, should use k0s from package
			c.Opts.Config = "/path/to/config.yaml"
			mockPackageManager.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/workdir/test-package/deps/kubernetes/files/k0s")
			mockK0sManager.EXPECT().Install("/path/to/config.yaml", "/test/workdir/test-package/deps/kubernetes/files/k0s", false).Return(nil)

			err := c.InstallK0s(mockPackageManager, mockK0sManager)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
