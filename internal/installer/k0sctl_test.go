// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("K0sctl", func() {
	var (
		mockEnv        *env.MockEnv
		mockHTTP       *portal.MockHttp
		mockFileWriter *util.MockFileIO
		cacheDir       string
		cachedPath     string
		k0sctl         *installer.K0sctl
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockHTTP = portal.NewMockHttp(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())
		cacheDir = GinkgoT().TempDir()
		cachedPath = filepath.Join(cacheDir, "k0sctl")

		mockEnv.EXPECT().GetOmsCacheDir().Return(cacheDir, nil)
		mockFileWriter.EXPECT().MkdirAll(cacheDir, os.FileMode(0755)).Return(nil)

		k0sctl = installer.NewK0sctl(mockHTTP, mockEnv, mockFileWriter)
		k0sctl.Goos = "linux"
		k0sctl.Goarch = "amd64"
	})

	writeCachedVersion := func(version string) {
		script := "#!/bin/sh\nprintf 'version: " + version + "\\ncommit: test\\n'\n"
		Expect(os.WriteFile(cachedPath, []byte(script), 0755)).To(Succeed())
	}

	expectDownload := func(version string) {
		url := "https://github.com/k0sproject/k0sctl/releases/download/" + version + "/k0sctl-linux-amd64"
		downloadFile, err := os.CreateTemp(cacheDir, "k0sctl-download")
		Expect(err).NotTo(HaveOccurred())
		mockFileWriter.EXPECT().Create(cachedPath).Return(downloadFile, nil)
		mockHTTP.EXPECT().Download(url, downloadFile, false).Return(nil)
		mockFileWriter.EXPECT().Chmod(cachedPath, os.FileMode(0755)).Return(nil)
	}

	It("reuses a cached binary with the requested version", func() {
		writeCachedVersion("v0.32.1")
		mockFileWriter.EXPECT().Exists(cachedPath).Return(true)

		path, err := k0sctl.Download("v0.32.1", false, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(cachedPath))
	})

	It("replaces a cached binary with a different version without force", func() {
		writeCachedVersion("v0.31.0")
		mockFileWriter.EXPECT().Exists(cachedPath).Return(true)
		expectDownload("v0.32.1")

		path, err := k0sctl.Download("v0.32.1", false, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(cachedPath))
	})

	It("resolves an unpinned version and reuses the matching binary", func() {
		writeCachedVersion("v0.32.1")
		mockHTTP.EXPECT().Get("https://api.github.com/repos/k0sproject/k0sctl/releases/latest").
			Return([]byte(`{"tag_name":"v0.32.1"}`), nil)
		mockFileWriter.EXPECT().Exists(cachedPath).Return(true)

		path, err := k0sctl.Download("", false, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(cachedPath))
	})
})
