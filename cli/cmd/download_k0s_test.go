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

var _ = Describe("DownloadK0sCmd", func() {
	var (
		c              cmd.DownloadK0sCmd
		opts           *cmd.DownloadK0sOpts
		globalOpts     *cmd.GlobalOptions
		mockEnv        *env.MockEnv
		mockFileWriter *util.MockFileIO
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.DownloadK0sOpts{
			GlobalOptions: globalOpts,
			Version:       "",
			Force:         false,
			Quiet:         false,
		}
		c = cmd.DownloadK0sCmd{
			Opts:       *opts,
			Env:        mockEnv,
			FileWriter: mockFileWriter,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockFileWriter.AssertExpectations(GinkgoT())
	})

	Context("DownloadK0s method", func() {
		It("fails when k0s manager fails to get latest version", func() {
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Version = "" // Test auto-version detection
			mockK0sManager.EXPECT().GetLatestVersion().Return("", errors.New("network error"))

			err := c.DownloadK0s(mockK0sManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get latest k0s version"))
			Expect(err.Error()).To(ContainSubstring("network error"))
		})

		It("fails when k0s manager fails to download", func() {
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Version = "v1.29.1+k0s.0"
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", false, false).Return("", errors.New("download failed"))

			err := c.DownloadK0s(mockK0sManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
			Expect(err.Error()).To(ContainSubstring("download failed"))
		})

		It("succeeds when version is specified and download works", func() {
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Version = "v1.29.1+k0s.0"
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", false, false).Return("/test/workdir/k0s", nil)

			err := c.DownloadK0s(mockK0sManager)
			Expect(err).ToNot(HaveOccurred())
		})

		It("succeeds when version is auto-detected and download works", func() {
			mockK0sManager := installer.NewMockK0sManager(GinkgoT())

			c.Opts.Version = "" // Test auto-version detection
			c.Opts.Force = true
			c.Opts.Quiet = true
			mockK0sManager.EXPECT().GetLatestVersion().Return("v1.29.1+k0s.0", nil)
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", true, true).Return("/test/workdir/k0s", nil)

			err := c.DownloadK0s(mockK0sManager)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
