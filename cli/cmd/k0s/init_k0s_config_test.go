// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/prompt"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("InitK0sConfigCmd", func() {
	var (
		c              k0s.InitK0sConfigCmd
		mockEnv        *env.MockEnv
		mockFileWriter *intutil.MockFileIO
		mockPM         *installer.MockPackageManager
		mockK0s        *installer.MockK0sManager
		mockPrompter   *prompt.MockPrompter
		tempDir        string
		defaultPath    string
	)

	// isAirgapConfig matches k0sctl configs that upload the bundle to the workers and stop
	// k0s from pulling images.
	isAirgapConfig := mock.MatchedBy(func(data []byte) bool {
		return strings.Contains(string(data), "dstDir: /var/lib/k0s/images") &&
			strings.Contains(string(data), "default_pull_policy: Never")
	})

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = intutil.NewMockFileIO(GinkgoT())
		mockPM = installer.NewMockPackageManager(GinkgoT())
		mockK0s = installer.NewMockK0sManager(GinkgoT())
		mockPrompter = prompt.NewMockPrompter(GinkgoT())
		tempDir = GinkgoT().TempDir()
		defaultPath = filepath.Join(tempDir, "k0sctl-config-test-dc.yaml")

		config := newTestInstallConfig(true)
		config.Kubernetes.Workers = []files.K8sNode{{IPAddress: "192.168.1.101"}}

		c = k0s.InitK0sConfigCmd{
			Opts: k0s.InitK0sConfigOpts{
				GlobalOptions: &util.GlobalOptions{},
				K0sctlConfigOpts: installer.K0sctlConfigOpts{
					InstallConfig: writeTestInstallConfig(tempDir, config),
					Version:       "v1.30.0+k0s.0",
				},
			},
			Env:        mockEnv,
			FileWriter: mockFileWriter,
			Prompter:   mockPrompter,
		}
	})

	cacheOnly := installer.DownloadOptions{CacheOnly: true}

	overwriteQuestion := mock.MatchedBy(func(question string) bool {
		return strings.Contains(question, "already exists")
	})

	setupWorkdirMocks := func() {
		mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
		mockFileWriter.EXPECT().MkdirAll(tempDir, os.FileMode(0755)).Return(nil)
	}

	It("writes the k0sctl config to the OMS workdir by default", func() {
		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().WriteFile(defaultPath, mock.Anything, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("overwrites an existing k0sctl config after confirmation", func() {
		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(true)
		mockPrompter.EXPECT().Bool(overwriteQuestion, false).Return(true)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().WriteFile(defaultPath, mock.Anything, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("keeps an existing k0sctl config when overwriting is declined", func() {
		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(true)
		mockPrompter.EXPECT().Bool(overwriteQuestion, false).Return(false)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("already exists")))
	})

	It("overwrites an existing k0sctl config without asking with --yes", func() {
		c.Opts.Yes = true

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().WriteFile(defaultPath, mock.Anything, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("writes an airgap k0sctl config to --output with the bundle from the OMS cache", func() {
		c.Opts.Airgap = true
		c.Opts.Output = "/etc/codesphere/k0sctl-config.yaml"

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists("/etc/codesphere/k0sctl-config.yaml").Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockK0s.EXPECT().DownloadAirgapBundle("v1.30.0+k0s.0", "", cacheOnly).Return("/cache/bundle.tar", nil)
		mockFileWriter.EXPECT().WriteFile("/etc/codesphere/k0sctl-config.yaml", isAirgapConfig, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("uses the airgap bundle given with --path", func() {
		c.Opts.Airgap = true
		c.Opts.AirgapBundlePath = "/opt/k0s/bundle.tar"

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().Exists("/opt/k0s/bundle.tar").Return(true)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.MatchedBy(func(data []byte) bool {
			return strings.Contains(string(data), "src: /opt/k0s/bundle.tar")
		}), os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("writes relative --path and --ssh-key-path as absolute paths", func() {
		c.Opts.Airgap = true
		c.Opts.AirgapBundlePath = "bundle.tar"
		c.Opts.SSHKeyPath = "keys/id_rsa"
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		bundlePath := filepath.Join(cwd, "bundle.tar")

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().Exists(bundlePath).Return(true)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.MatchedBy(func(data []byte) bool {
			return strings.Contains(string(data), "src: "+bundlePath) &&
				strings.Contains(string(data), "keyPath: "+filepath.Join(cwd, "keys/id_rsa"))
		}), os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("keeps an SSH key path starting with ~ for k0sctl to expand", func() {
		c.Opts.SSHKeyPath = "~/.ssh/id_rsa"

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.MatchedBy(func(data []byte) bool {
			return strings.Contains(string(data), "keyPath: ~/.ssh/id_rsa")
		}), os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("takes the k0s binary from the package but still uses the airgap bundle", func() {
		c.Opts.Airgap = true
		c.Opts.Package = "test-package.tar.gz"

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false, false).Return(nil)
		mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
		mockK0s.EXPECT().DownloadAirgapBundle("v1.30.0+k0s.0", "", cacheOnly).Return("/cache/bundle.tar", nil)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, isAirgapConfig, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("fails when the airgap bundle given with --path does not exist", func() {
		c.Opts.Airgap = true
		c.Opts.AirgapBundlePath = "/nonexistent/bundle.tar"

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().Exists("/nonexistent/bundle.tar").Return(false)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("does not exist")))
	})

	It("does not download a missing k0s binary", func() {
		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("", errors.New("k0s v1.30.0+k0s.0 not found in OMS cache"))

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("not found in OMS cache")))
	})

	It("does not download a missing airgap bundle", func() {
		c.Opts.Airgap = true

		setupWorkdirMocks()
		mockFileWriter.EXPECT().Exists(defaultPath).Return(false)
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockK0s.EXPECT().DownloadAirgapBundle("v1.30.0+k0s.0", "", cacheOnly).Return("", errors.New("k0s airgap bundle v1.30.0+k0s.0 not found"))

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("airgap bundle v1.30.0+k0s.0 not found")))
	})

	It("fails when --path is given without --airgapped", func() {
		c.Opts.AirgapBundlePath = "/opt/k0s/bundle.tar"

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError("--path requires --airgapped"))
	})

	It("fails when --no-download is combined with --airgapped", func() {
		c.Opts.Airgap = true
		c.Opts.NoDownload = true

		setupWorkdirMocks()

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("--no-download cannot be combined with --airgapped")))
	})

	It("fails without install-config", func() {
		c.Opts.InstallConfig = ""

		setupWorkdirMocks()

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("--install-config is required")))
	})
})
