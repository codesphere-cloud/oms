// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package tmpl_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/tmpl"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("GenerateDockerfile", func() {
	var (
		tempDir    string
		mockFileIO *util.MockFileIO
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "tmpl-test-*")
		Expect(err).To(BeNil())

		mockFileIO = util.NewMockFileIO(GinkgoT())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		if mockFileIO != nil {
			mockFileIO.AssertExpectations(GinkgoT())
		}
	})

	Context("with valid inputs using real FileIO", func() {
		It("should generate a Dockerfile with a correct base image", func() {
			outputPath := tempDir + "/Dockerfile"
			baseImage := "alpine:3.18"

			err := tmpl.GenerateDockerfile(&util.FilesystemWriter{}, outputPath, baseImage)
			Expect(err).To(BeNil())

			content, err := os.ReadFile(outputPath)
			Expect(err).To(BeNil())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("FROM alpine:3.18"))
		})

		It("should include all expected template sections", func() {
			outputPath := tempDir + "/Dockerfile"
			baseImage := "node:18-alpine"

			err := tmpl.GenerateDockerfile(&util.FilesystemWriter{}, outputPath, baseImage)
			Expect(err).To(BeNil())

			content, err := os.ReadFile(outputPath)
			Expect(err).To(BeNil())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("FROM node:18-alpine"))
			Expect(contentStr).To(ContainSubstring("USER root"))
			Expect(contentStr).To(ContainSubstring("USER user"))
			Expect(contentStr).To(ContainSubstring("### Install Dependencies as root ###"))
			Expect(contentStr).To(ContainSubstring("### Uncomment lines and add dependencies as necessary ###"))
			Expect(contentStr).To(ContainSubstring("### Run apt-get update, install specified packages, and clean up the cache. ###"))
			Expect(contentStr).To(ContainSubstring("### Switch to Non-Root User ###"))
		})

		It("should generate consistent output for the same input", func() {
			outputPath1 := tempDir + "/Dockerfile1"
			outputPath2 := tempDir + "/Dockerfile2"
			baseImage := "golang:1.21"

			err1 := tmpl.GenerateDockerfile(&util.FilesystemWriter{}, outputPath1, baseImage)
			err2 := tmpl.GenerateDockerfile(&util.FilesystemWriter{}, outputPath2, baseImage)

			Expect(err1).To(BeNil())
			Expect(err2).To(BeNil())

			content1, err := os.ReadFile(outputPath1)
			Expect(err).To(BeNil())
			content2, err := os.ReadFile(outputPath2)
			Expect(err).To(BeNil())

			Expect(string(content1)).To(Equal(string(content2)), "Generated Dockerfiles should be identical for the same input")
		})
	})

	Context("when file creation fails", func() {
		It("should return an error when Create fails", func() {
			outputPath := "/invalid/path/Dockerfile"
			baseImage := "ubuntu:20.04"

			mockFileIO.EXPECT().Create(outputPath).Return(nil, os.ErrNotExist)

			err := tmpl.GenerateDockerfile(mockFileIO, outputPath, baseImage)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error creating output file"))
			Expect(err.Error()).To(ContainSubstring(outputPath))
		})
	})
})
