// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("K0s", func() {
	var (
		k0s            installer.K0sManager
		k0sImpl        *installer.K0s
		mockEnv        *env.MockEnv
		mockHttp       *portal.MockHttp
		mockFileWriter *util.MockFileIO
		tempDir        string
		workDir        string
		k0sPath        string
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockHttp = portal.NewMockHttp(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())

		tempDir = GinkgoT().TempDir()
		workDir = filepath.Join(tempDir, "oms-workdir")
		k0sPath = filepath.Join(workDir, "k0s")

		k0s = installer.NewK0s(mockHttp, mockEnv, mockFileWriter)
		k0sImpl = k0s.(*installer.K0s)
	})

	Describe("NewK0s", func() {
		It("creates a new K0s with correct parameters", func() {
			newK0s := installer.NewK0s(mockHttp, mockEnv, mockFileWriter)
			Expect(newK0s).ToNot(BeNil())

			// Type assertion to access fields
			k0sStruct := newK0s.(*installer.K0s)
			Expect(k0sStruct.Http).To(Equal(mockHttp))
			Expect(k0sStruct.Env).To(Equal(mockEnv))
			Expect(k0sStruct.FileWriter).To(Equal(mockFileWriter))
			Expect(k0sStruct.Goos).ToNot(BeEmpty())
			Expect(k0sStruct.Goarch).ToNot(BeEmpty())
		})

		It("implements K0sManager interface", func() {
			manager := installer.NewK0s(mockHttp, mockEnv, mockFileWriter)
			Expect(manager).ToNot(BeNil())
		})
	})

	Describe("GetLatestVersion", func() {
		Context("when version fetch succeeds", func() {
			It("returns the latest version", func() {
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return([]byte("v1.29.1+k0s.0"), nil)

				version, err := k0s.GetLatestVersion()
				Expect(err).ToNot(HaveOccurred())
				Expect(version).To(Equal("v1.29.1+k0s.0"))
			})
		})

		Context("when version fetch fails", func() {
			It("returns an error", func() {
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return(nil, errors.New("network error"))

				_, err := k0s.GetLatestVersion()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch version info"))
				Expect(err.Error()).To(ContainSubstring("network error"))
			})
		})

		Context("when version is empty", func() {
			It("returns an error", func() {
				emptyVersionBytes := []byte("   \n  ")
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return(emptyVersionBytes, nil)

				_, err := k0s.GetLatestVersion()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("version info is empty"))
			})
		})

		Context("when version has whitespace", func() {
			It("trims whitespace correctly", func() {
				versionWithWhitespace := []byte("  v1.29.1+k0s.0  \n")
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return(versionWithWhitespace, nil)

				version, err := k0s.GetLatestVersion()
				Expect(err).ToNot(HaveOccurred())
				Expect(version).To(Equal("v1.29.1+k0s.0"))
			})
		})
	})

	Describe("Download", func() {
		Context("Platform support", func() {
			It("should fail on non-Linux platforms", func() {
				k0sImpl.Goos = "windows"
				k0sImpl.Goarch = "amd64"

				_, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("windows/amd64"))
			})

			It("should fail on non-amd64 architectures", func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "arm64"

				_, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("linux/arm64"))
			})
		})

		Context("Version fetching", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
			})

			It("should handle version parameter correctly", func() {
				mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
				mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(nil)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

				// Create the workdir first
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				// Create a real file for the test
				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())

				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)
				mockFileWriter.EXPECT().Chmod(k0sPath, os.FileMode(0755)).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})

		Context("File existence checks", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"

				mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
				mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(nil)
			})

			It("should reuse a cached k0s binary with the requested version", func() {
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())
				err = os.WriteFile(k0sPath, []byte("#!/bin/sh\nprintf 'v1.29.1+k0s.0\\n'\n"), 0755)
				Expect(err).ToNot(HaveOccurred())
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)

				path, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})

			It("should replace a cached k0s binary with a different version", func() {
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())
				err = os.WriteFile(k0sPath, []byte("#!/bin/sh\nprintf 'v1.28.0+k0s.0\\n'\n"), 0755)
				Expect(err).ToNot(HaveOccurred())
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)

				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())

				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)
				mockFileWriter.EXPECT().Chmod(k0sPath, os.FileMode(0755)).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})

			It("should proceed when k0s binary exists and force is true", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)

				// Create the workdir first
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				// Create a real file for the test
				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())

				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)
				mockFileWriter.EXPECT().Chmod(k0sPath, os.FileMode(0755)).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{Force: true})
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})

		Context("File operations", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"

				mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
				mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(nil)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)
			})

			It("should fail when file creation fails", func() {
				mockFileWriter.EXPECT().Create(k0sPath).Return(nil, errors.New("permission denied"))

				_, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to download k0s binary"))
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})

			It("should fail when download fails", func() {
				// Create a mock file for the test
				mockFile, err := os.CreateTemp("", "k0s-test")
				Expect(err).ToNot(HaveOccurred())

				defer func() {
					_ = os.Remove(mockFile.Name())
				}()
				defer util.CloseFileIgnoreError(mockFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(mockFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", mockFile, false).Return(errors.New("download failed"))
				// The truncated destination must not stay behind as a reusable cache entry.
				mockFileWriter.EXPECT().Remove(k0sPath).Return(nil)

				_, err = k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to download k0s binary"))
				Expect(err.Error()).To(ContainSubstring("download failed"))
			})

			It("should succeed with default options", func() {
				// Create a real file in temp directory for mock Create to return
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())

				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)
				mockFileWriter.EXPECT().Chmod(k0sPath, os.FileMode(0755)).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})

		Context("URL construction", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"

				mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)
			})

			It("should construct correct download URL for amd64", func() {
				k0sImpl.Goarch = "amd64"

				mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(nil)

				// Create the workdir first
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				// Create a real file for the test
				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())

				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)
				mockFileWriter.EXPECT().Chmod(k0sPath, os.FileMode(0755)).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", installer.DownloadOptions{})
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})
	})

	Describe("EnsureAirgapBundle", func() {
		const (
			legacyVersion = "v1.31.14+k0s.0"
			modernVersion = "v1.36.3+k0s.2"
		)

		var (
			legacyAsset = "k0s-airgap-bundle-" + legacyVersion + "-amd64"
			modernAsset = "k0s-airgap-bundle-" + modernVersion + "-linux-amd64.tar"
		)

		// expectAirgapDownload sets up the file and http mocks for a fresh download of
		// assetName and returns the expected bundle path. otherAssets are listed by the
		// release before assetName.
		expectAirgapDownload := func(version, assetName string, otherAssets ...string) string {
			bundlePath := filepath.Join(workDir, assetName)

			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, nil)
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+version).
				Return(releaseJSON(append(otherAssets, assetName)...), nil)

			err := os.MkdirAll(workDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			realFile, err := os.Create(bundlePath)
			Expect(err).ToNot(HaveOccurred())

			defer util.CloseFileIgnoreError(realFile)

			mockFileWriter.EXPECT().Exists(bundlePath).Return(false)
			mockFileWriter.EXPECT().Create(bundlePath).Return(realFile, nil)
			mockHttp.EXPECT().Download(
				"https://github.com/k0sproject/k0s/releases/download/"+version+"/"+assetName, realFile, false,
			).Return(nil)

			return bundlePath
		}

		BeforeEach(func() {
			k0sImpl.Goos = "linux"
			k0sImpl.Goarch = "amd64"

			mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
			mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(nil)
		})

		It("resolves the legacy asset name used up to k0s v1.35", func() {
			bundlePath := expectAirgapDownload(legacyVersion, legacyAsset)

			path, err := k0s.EnsureAirgapBundle(legacyVersion, installer.DownloadOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("resolves the current asset name with OS and extension used since k0s v1.36", func() {
			bundlePath := expectAirgapDownload(modernVersion, modernAsset)

			path, err := k0s.EnsureAirgapBundle(modernVersion, installer.DownloadOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("accepts the legacy asset name with a tar extension", func() {
			bundlePath := expectAirgapDownload(legacyVersion, legacyAsset+".tar")

			path, err := k0s.EnsureAirgapBundle(legacyVersion, installer.DownloadOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("ignores assets of other platforms", func() {
			bundlePath := expectAirgapDownload(
				modernVersion,
				modernAsset,
				"k0s-airgap-bundle-"+modernVersion+"-linux-arm64.tar",
				"k0s-airgap-bundle-"+modernVersion+"-windows2022-amd64.tar",
			)

			path, err := k0s.EnsureAirgapBundle(modernVersion, installer.DownloadOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("reuses a cached bundle without contacting the release API", func() {
			bundlePath := filepath.Join(workDir, modernAsset)

			mockFileWriter.EXPECT().ReadDir(workDir).Return([]os.DirEntry{fakeDirEntry{name: modernAsset}}, nil)
			mockFileWriter.EXPECT().Exists(bundlePath).Return(true)

			path, err := k0s.EnsureAirgapBundle(modernVersion, installer.DownloadOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("re-downloads a cached bundle when force is set", func() {
			bundlePath := filepath.Join(workDir, modernAsset)

			mockFileWriter.EXPECT().ReadDir(workDir).Return([]os.DirEntry{fakeDirEntry{name: modernAsset}}, nil)

			err := os.MkdirAll(workDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			realFile, err := os.Create(bundlePath)
			Expect(err).ToNot(HaveOccurred())

			defer util.CloseFileIgnoreError(realFile)

			mockFileWriter.EXPECT().Exists(bundlePath).Return(true)
			mockFileWriter.EXPECT().Create(bundlePath).Return(realFile, nil)
			mockHttp.EXPECT().Download(
				"https://github.com/k0sproject/k0s/releases/download/"+modernVersion+"/"+modernAsset, realFile, false,
			).Return(nil)

			path, err := k0s.EnsureAirgapBundle(modernVersion, installer.DownloadOptions{Force: true})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("falls back to the release API when the cache cannot be read", func() {
			bundlePath := filepath.Join(workDir, modernAsset)

			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, errors.New("permission denied"))
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+modernVersion).
				Return(releaseJSON(modernAsset), nil)

			err := os.MkdirAll(workDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			realFile, err := os.Create(bundlePath)
			Expect(err).ToNot(HaveOccurred())

			defer util.CloseFileIgnoreError(realFile)

			mockFileWriter.EXPECT().Exists(bundlePath).Return(false)
			mockFileWriter.EXPECT().Create(bundlePath).Return(realFile, nil)
			mockHttp.EXPECT().Download(
				"https://github.com/k0sproject/k0s/releases/download/"+modernVersion+"/"+modernAsset, realFile, false,
			).Return(nil)

			path, err := k0s.EnsureAirgapBundle(modernVersion, installer.DownloadOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(bundlePath))
		})

		It("removes a partial bundle when the transfer fails", func() {
			bundlePath := filepath.Join(workDir, modernAsset)

			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, nil)
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+modernVersion).
				Return(releaseJSON(modernAsset), nil)

			err := os.MkdirAll(workDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			realFile, err := os.Create(bundlePath)
			Expect(err).ToNot(HaveOccurred())

			defer util.CloseFileIgnoreError(realFile)

			mockFileWriter.EXPECT().Exists(bundlePath).Return(false)
			mockFileWriter.EXPECT().Create(bundlePath).Return(realFile, nil)
			mockHttp.EXPECT().Download(
				"https://github.com/k0sproject/k0s/releases/download/"+modernVersion+"/"+modernAsset, realFile, false,
			).Return(errors.New("connection reset"))
			// The cache only checks for existence, so the partial bundle must go.
			mockFileWriter.EXPECT().Remove(bundlePath).Return(nil)

			_, err = k0s.EnsureAirgapBundle(modernVersion, installer.DownloadOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download"))
		})

		It("fails when the release metadata cannot be fetched", func() {
			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, nil)
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+legacyVersion).
				Return(nil, errors.New("network error"))

			_, err := k0s.EnsureAirgapBundle(legacyVersion, installer.DownloadOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to fetch k0s release"))
		})

		It("fails when the release has no airgap bundle for the platform", func() {
			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, nil)
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+legacyVersion).
				Return(releaseJSON("k0s-v1.31.14+k0s.0-amd64"), nil)

			_, err := k0s.EnsureAirgapBundle(legacyVersion, installer.DownloadOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no airgap bundle for linux/amd64"))
		})
	})

	Describe("cache failures", func() {
		BeforeEach(func() {
			k0sImpl.Goos = "linux"
			k0sImpl.Goarch = "amd64"
		})

		It("fails when the cache directory cannot be determined", func() {
			mockEnv.EXPECT().GetOmsCacheDir().Return("", errors.New("no cache dir"))

			_, err := k0s.EnsureAirgapBundle("v1.31.14+k0s.0", installer.DownloadOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to determine cache directory"))
		})

		It("fails when the cache directory cannot be created", func() {
			mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
			mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(errors.New("permission denied"))

			_, err := k0s.EnsureAirgapBundle("v1.31.14+k0s.0", installer.DownloadOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create workdir"))
		})
	})

	Describe("Download with airgap", func() {
		const airgapVersion = "v1.36.3+k0s.2"

		var airgapAsset = "k0s-airgap-bundle-" + airgapVersion + "-linux-amd64.tar"

		// expectBinaryDownload sets up the mocks for a fresh k0s binary download.
		expectBinaryDownload := func() {
			mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

			err := os.MkdirAll(workDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			realFile, err := os.Create(k0sPath)
			Expect(err).ToNot(HaveOccurred())

			defer util.CloseFileIgnoreError(realFile)

			mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
			mockHttp.EXPECT().Download(
				"https://github.com/k0sproject/k0s/releases/download/"+airgapVersion+"/k0s-"+airgapVersion+"-amd64", realFile, false,
			).Return(nil)
			mockFileWriter.EXPECT().Chmod(k0sPath, os.FileMode(0755)).Return(nil)
		}

		BeforeEach(func() {
			k0sImpl.Goos = "linux"
			k0sImpl.Goarch = "amd64"

			mockEnv.EXPECT().GetOmsCacheDir().Return(workDir, nil)
			mockFileWriter.EXPECT().MkdirAll(workDir, os.FileMode(0755)).Return(nil)
		})

		It("downloads the airgap bundle in addition to the binary", func() {
			expectBinaryDownload()

			bundlePath := filepath.Join(workDir, airgapAsset)

			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, nil)
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+airgapVersion).
				Return(releaseJSON(airgapAsset), nil)

			bundleFile, err := os.Create(bundlePath)
			Expect(err).ToNot(HaveOccurred())

			defer util.CloseFileIgnoreError(bundleFile)

			mockFileWriter.EXPECT().Exists(bundlePath).Return(false)
			mockFileWriter.EXPECT().Create(bundlePath).Return(bundleFile, nil)
			mockHttp.EXPECT().Download(
				"https://github.com/k0sproject/k0s/releases/download/"+airgapVersion+"/"+airgapAsset, bundleFile, false,
			).Return(nil)

			path, err := k0s.Download(airgapVersion, installer.DownloadOptions{Airgapped: true})
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal(k0sPath))
		})

		It("wraps airgap bundle download failures and returns no binary path", func() {
			expectBinaryDownload()

			mockFileWriter.EXPECT().ReadDir(workDir).Return(nil, nil)
			mockHttp.EXPECT().Get("https://api.github.com/repos/k0sproject/k0s/releases/tags/"+airgapVersion).
				Return(nil, errors.New("network error"))

			path, err := k0s.Download(airgapVersion, installer.DownloadOptions{Airgapped: true})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s airgap bundle"))
			Expect(err.Error()).To(ContainSubstring("network error"))
			Expect(path).To(BeEmpty())
		})
	})
})

// fakeDirEntry is a minimal os.DirEntry for cache lookups in tests.
type fakeDirEntry struct {
	name string
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() os.FileMode          { return 0 }
func (f fakeDirEntry) Info() (os.FileInfo, error) { return nil, nil }

// releaseJSON renders a GitHub release API response listing the given assets.
func releaseJSON(names ...string) []byte {
	type asset struct {
		Name string `json:"name"`
	}

	assets := make([]asset, 0, len(names))
	for _, name := range names {
		assets = append(assets, asset{Name: name})
	}

	data, err := json.Marshal(struct {
		Assets []asset `json:"assets"`
	}{Assets: assets})
	Expect(err).ToNot(HaveOccurred())

	return data
}
