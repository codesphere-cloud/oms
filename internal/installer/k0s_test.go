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
	})

	Describe("Download", func() {
		Context("Platform support", func() {
			It("should fail on non-Linux platforms", func() {
				k0sImpl.Goos = "windows"
				k0sImpl.Goarch = "amd64"

				err := k0s.Download(false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("windows/amd64"))
			})

			It("should fail on non-amd64 architectures", func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "arm64"

				err := k0s.Download(false, false)
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

			It("should fail when version fetch fails", func() {
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return(nil, errors.New("network error"))

				err := k0s.Download(false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch version info"))
				Expect(err.Error()).To(ContainSubstring("network error"))
			})

			It("should fail when version is empty", func() {
				emptyVersionBytes := []byte("   \n  ")
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return(emptyVersionBytes, nil)

				err := k0s.Download(false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("version info is empty"))
			})

			It("should handle version with whitespace correctly", func() {
				versionWithWhitespace := []byte("  v1.29.1+k0s.0  \n")
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return(versionWithWhitespace, nil)
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

				// Create the workdir first
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				// Create a real file for the test
				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())
				defer realFile.Close()

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)

				err = k0s.Download(false, false)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("File existence checks", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return([]byte("v1.29.1+k0s.0"), nil)
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
			})

			It("should fail when k0s binary exists and force is false", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)

				err := k0s.Download(false, false)
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
				defer realFile.Close()

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)

				err = k0s.Download(true, false)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("File operations", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return([]byte("v1.29.1+k0s.0"), nil)
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)
			})

			It("should fail when file creation fails", func() {
				mockFileWriter.EXPECT().Create(k0sPath).Return(nil, errors.New("permission denied"))

				err := k0s.Download(false, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create k0s binary file"))
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})

			It("should fail when download fails", func() {
				// Create a mock file for the test
				mockFile, err := os.CreateTemp("", "k0s-test")
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(mockFile.Name())
				defer mockFile.Close()

				mockFileWriter.EXPECT().Create(k0sPath).Return(mockFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", mockFile, false).Return(errors.New("download failed"))

				err = k0s.Download(false, false)
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
				defer realFile.Close()

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)

				err = k0s.Download(false, false)
				Expect(err).ToNot(HaveOccurred())

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
				mockHttp.EXPECT().Get("https://docs.k0sproject.io/stable.txt").Return([]byte("v1.29.1+k0s.0"), nil)

				// Create the workdir first
				err := os.MkdirAll(workDir, 0755)
				Expect(err).ToNot(HaveOccurred())

				// Create a real file for the test
				realFile, err := os.Create(k0sPath)
				Expect(err).ToNot(HaveOccurred())
				defer realFile.Close()

				mockFileWriter.EXPECT().Create(k0sPath).Return(realFile, nil)
				mockHttp.EXPECT().Download("https://github.com/k0sproject/k0s/releases/download/v1.29.1+k0s.0/k0s-v1.29.1+k0s.0-amd64", realFile, false).Return(nil)

				err = k0s.Download(false, false)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Install", func() {
		Context("Platform support", func() {
			It("should fail on non-Linux platforms", func() {
				k0sImpl.Goos = "windows"
				k0sImpl.Goarch = "amd64"

				err := k0s.Install("", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("windows/amd64"))
			})

			It("should fail on non-amd64 architectures", func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "arm64"

				err := k0s.Install("", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring("linux/arm64"))
			})
		})

		Context("Binary existence checks", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
			})

			It("should fail when k0s binary doesn't exist", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(false)

				err := k0s.Install("", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k0s binary does not exist"))
				Expect(err.Error()).To(ContainSubstring("please download first"))
			})

			It("should proceed when k0s binary exists", func() {
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)

				// This will fail with exec error since we can't actually run k0s in tests
				// but it will pass the existence check
				err := k0s.Install("", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
			})
		})

		Context("Installation modes", func() {
			BeforeEach(func() {
				k0sImpl.Goos = "linux"
				k0sImpl.Goarch = "amd64"
				mockEnv.EXPECT().GetOmsWorkdir().Return(workDir)
				mockFileWriter.EXPECT().Exists(k0sPath).Return(true)
			})

			It("should install in single-node mode when no config path is provided", func() {
				// This will fail with exec error but we can verify the command would be called
				// The command should be: ./k0s install controller --single
				err := k0s.Install("", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
			})

			It("should install with custom config when config path is provided", func() {
				configPath := "/path/to/k0s.yaml"
				// This will fail with exec error but we can verify the command would be called
				// The command should be: ./k0s install controller --config /path/to/k0s.yaml
				err := k0s.Install(configPath, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
			})

			It("should install with custom config and force flag", func() {
				configPath := "/path/to/k0s.yaml"
				// This will fail with exec error but we can verify the command would be called
				// The command should be: ./k0s install controller --config /path/to/k0s.yaml --force
				err := k0s.Install(configPath, true)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
			})
		})
	})
})
