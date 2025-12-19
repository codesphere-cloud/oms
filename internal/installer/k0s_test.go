// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
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
			var manager = installer.NewK0s(mockHttp, mockEnv, mockFileWriter)
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

				_, err := k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("windows/amd64"))
			})

			It("should fail on non-amd64 architectures", func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "arm64"

				_, err := k0s.Download("v1.29.1+k0s.0", false, false)
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
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
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

				path, err := k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})

		Context("File existence checks", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
			})

			It("should fail when k0s binary exists and force is false", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)

				_, err := k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k0s binary already exists"))
				Expect(err.Error()).To(ContainSubstring("Use --force to overwrite"))
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

				path, err := k0s.Download("v1.29.1+k0s.0", true, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})

		Context("File operations", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)
			})

			It("should fail when file creation fails", func() {
				mockFileWriter.EXPECT().Create(k0sPath).Return(nil, errors.New("permission denied"))

				_, err := k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create k0s binary file"))
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

				_, err = k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to download k0s binary"))
				Expect(err.Error()).To(ContainSubstring("download failed"))
			})

			It("should succeed with default options", func() {
				// Create a real file in temp directory for os.Chmod to work
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())
				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))

				// Verify file was made executable
				info, err := os.Stat(k0sPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Mode() & 0755).To(Equal(os.FileMode(0755)))
			})
		})

		Context("URL construction", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)
			})

			It("should construct correct download URL for amd64", func() {
				k0sImpl.Goarch = "amd64"

				// Create the workdir first
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				// Create a real file for the test
				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())
				defer util.CloseFileIgnoreError(realFile)

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)

				path, err := k0s.Download("v1.29.1+k0s.0", false, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(k0sPath))
			})
		})
	})

	Describe("Install", func() {
		Context("Platform support", func() {
			It("should fail on non-Linux platforms", func() {
				k0sImpl.Goos = "windows"
				k0sImpl.Goarch = "amd64"

				err := k0s.Install("", k0sPath, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k0s installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("windows/amd64"))
			})

			It("should fail on non-amd64 architectures", func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "arm64"

				err := k0s.Install("", k0sPath, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k0s installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("linux/arm64"))
			})
		})

		Context("Binary existence checks", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
			})

			It("should fail when k0s binary doesn't exist", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

				err := k0s.Install("", k0sPath, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k0s binary does not exist"))
				Expect(err.Error()).To(ContainSubstring("please download first"))
			})
		})
	})

	Describe("Reset", func() {
		BeforeEach(func() {
			k0sImpl.Goos = "linux"
			k0sImpl.Goarch = "amd64"
		})

		Context("when k0s binary does not exist", func() {
			It("should return nil without attempting reset", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

				err := k0s.Reset(k0sPath)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("platform validation", func() {
			It("should work regardless of platform for reset", func() {
				k0sImpl.Goos = "darwin"
				k0sImpl.Goarch = "arm64"

				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

				err := k0s.Reset(k0sPath)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
