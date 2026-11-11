// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("DownloadK0sk0s", func() {
	var (
		c              k0s.DownloadK0sCmd
		opts           *k0s.DownloadK0sOpts
		globalOpts     *util.GlobalOptions
		mockEnv        *env.MockEnv
		mockFileWriter *intutil.MockFileIO
		mockK0sManager *installer.MockK0sManager
		mockK0sctl     *installer.MockK0sctlManager
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = intutil.NewMockFileIO(GinkgoT())
		mockK0sManager = installer.NewMockK0sManager(GinkgoT())
		mockK0sctl = installer.NewMockK0sctlManager(GinkgoT())
		globalOpts = &util.GlobalOptions{}
		opts = &k0s.DownloadK0sOpts{
			GlobalOptions: globalOpts,
			Version:       "",
			K0sctlVersion: "v0.33.1",
			Force:         false,
		}
		c = k0s.DownloadK0sCmd{
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
			c.Opts.Version = "" // Test auto-version detection

			mockK0sManager.EXPECT().GetLatestVersion().Return("", errors.New("network error"))

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get latest k0s version"))
			Expect(err.Error()).To(ContainSubstring("network error"))
		})

		It("fails when k0s manager fails to download", func() {
			c.Opts.Version = "v1.29.1+k0s.0"

			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", installer.DownloadOptions{Quiet: true}).Return("", errors.New("download failed"))

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
			Expect(err.Error()).To(ContainSubstring("download failed"))
		})

		It("succeeds when version is specified and download works", func() {
			c.Opts.Version = "v1.29.1+k0s.0"

			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", installer.DownloadOptions{Quiet: true}).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.33.1", installer.DownloadOptions{Quiet: true}).Return("/cache/k0sctl", nil)

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).ToNot(HaveOccurred())
		})

		It("downloads k0sctl in the requested version", func() {
			c.Opts.Version = "v1.29.1+k0s.0"
			c.Opts.K0sctlVersion = "v0.30.0"

			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", installer.DownloadOptions{Quiet: true}).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.30.0", installer.DownloadOptions{Quiet: true}).Return("/cache/k0sctl", nil)

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).ToNot(HaveOccurred())
		})

		It("reports k0sctl download failures before downloading the airgap bundle", func() {
			c.Opts.Version = "v1.29.1+k0s.0"
			c.Opts.Airgap = true

			opts := installer.DownloadOptions{Quiet: true}
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", opts).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.33.1", opts).Return("", errors.New("network error"))

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0sctl"))
			Expect(err.Error()).To(ContainSubstring("network error"))
		})

		It("requests the airgap bundle when --airgapped is set", func() {
			c.Opts.Version = "v1.29.1+k0s.0"
			c.Opts.Airgap = true

			opts := installer.DownloadOptions{Quiet: true}
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", opts).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.33.1", opts).Return("/cache/k0sctl", nil)
			mockK0sManager.EXPECT().DownloadAirgapBundle("v1.29.1+k0s.0", "", opts).Return("/cache/k0s-airgap-bundle-amd64", nil)

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).ToNot(HaveOccurred())
		})

		It("saves the airgap bundle to --path", func() {
			c.Opts.Version = "v1.29.1+k0s.0"
			c.Opts.Airgap = true
			c.Opts.Path = "/opt/k0s/bundle.tar"

			opts := installer.DownloadOptions{Quiet: true}
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", opts).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.33.1", opts).Return("/cache/k0sctl", nil)
			mockK0sManager.EXPECT().DownloadAirgapBundle("v1.29.1+k0s.0", "/opt/k0s/bundle.tar", opts).Return("/opt/k0s/bundle.tar", nil)

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).ToNot(HaveOccurred())
		})

		It("fails when --path is given without --airgapped", func() {
			c.Opts.Path = "/opt/k0s/bundle.tar"

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).To(MatchError("--path requires --airgapped"))
		})

		It("reports airgap bundle download failures", func() {
			c.Opts.Version = "v1.29.1+k0s.0"
			c.Opts.Airgap = true

			opts := installer.DownloadOptions{Quiet: true}
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", opts).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.33.1", opts).Return("/cache/k0sctl", nil)
			mockK0sManager.EXPECT().DownloadAirgapBundle("v1.29.1+k0s.0", "", opts).Return("", errors.New("network error"))

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s airgap bundle"))
			Expect(err.Error()).To(ContainSubstring("network error"))
		})

		It("succeeds when version is auto-detected and download works", func() {
			c.Opts.Version = "" // Test auto-version detection
			c.Opts.Force = true
			c.Opts.Verbose = true

			mockK0sManager.EXPECT().GetLatestVersion().Return("v1.29.1+k0s.0", nil)
			mockK0sManager.EXPECT().Download("v1.29.1+k0s.0", installer.DownloadOptions{Force: true, Quiet: false}).Return("/test/workdir/k0s", nil)
			mockK0sctl.EXPECT().Download("v0.33.1", installer.DownloadOptions{Force: true, Quiet: false}).Return("/cache/k0sctl", nil)

			err := c.DownloadK0s(mockK0sManager, mockK0sctl)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
