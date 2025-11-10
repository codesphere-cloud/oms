// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("Docker", func() {
	var dockerfileManager util.DockerfileManager

	BeforeEach(func() {
		dockerfileManager = util.NewDockerfileManager()
	})

	Describe("UpdateFromStatement", func() {
		It("updates a simple FROM statement with workspace-agent", func() {
			dockerfileContent := `FROM workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			// Create a temporary file with the dockerfile content
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = tempFile.WriteString(dockerfileContent)
			Expect(err).NotTo(HaveOccurred())

			err = dockerfileManager.UpdateFromStatement(tempFile.Name(), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())

			// Read the updated content and verify
			tempFile.Seek(0, 0)
			updatedContent, err := io.ReadAll(tempFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(updatedContent)).To(Equal(`FROM workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("updates FROM statement with workspace-agent and platform flag", func() {
			dockerfileContent := `FROM --platform=linux/amd64 workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = tempFile.WriteString(dockerfileContent)
			Expect(err).NotTo(HaveOccurred())

			err = dockerfileManager.UpdateFromStatement(tempFile.Name(), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())

			tempFile.Seek(0, 0)
			updatedContent, err := io.ReadAll(tempFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(updatedContent)).To(Equal(`FROM workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("handles case-insensitive FROM statement with workspace-agent", func() {
			dockerfileContent := `from workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = tempFile.WriteString(dockerfileContent)
			Expect(err).NotTo(HaveOccurred())

			err = dockerfileManager.UpdateFromStatement(tempFile.Name(), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())

			tempFile.Seek(0, 0)
			updatedContent, err := io.ReadAll(tempFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(updatedContent)).To(Equal(`from workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("handles FROM with complex workspace-agent image names", func() {
			dockerfileContent := `FROM registry.example.com:5000/workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = tempFile.WriteString(dockerfileContent)
			Expect(err).NotTo(HaveOccurred())

			err = dockerfileManager.UpdateFromStatement(tempFile.Name(), "new-registry.com/workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())

			tempFile.Seek(0, 0)
			updatedContent, err := io.ReadAll(tempFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(updatedContent)).To(Equal(`FROM new-registry.com/workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("updates the last FROM statement in multi-stage builds", func() {
			dockerfileContent := `FROM alpine:3.14
RUN apt-get update
FROM workspace-agent-24.04:20.04 as builder
COPY --from=0 /app /app`

			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = tempFile.WriteString(dockerfileContent)
			Expect(err).NotTo(HaveOccurred())

			err = dockerfileManager.UpdateFromStatement(tempFile.Name(), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())

			tempFile.Seek(0, 0)
			updatedContent, err := io.ReadAll(tempFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(updatedContent)).To(Equal(`FROM alpine:3.14
RUN apt-get update
FROM workspace-agent-24.04:codesphere-v1.0.1 as builder
COPY --from=0 /app /app`))
		})

		It("returns error when no FROM statement with workspace-agent found", func() {
			dockerfileContent := `FROM ubuntu:20.04
RUN apt-get update
WORKDIR /app`

			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = tempFile.WriteString(dockerfileContent)
			Expect(err).NotTo(HaveOccurred())

			err = dockerfileManager.UpdateFromStatement(tempFile.Name(), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no FROM statement with workspace-agent found in dockerfile"))
		})

		It("returns error when dockerfile does not exist", func() {
			err := dockerfileManager.UpdateFromStatement("nonexistent-dockerfile", "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to open dockerfile"))
		})
	})
})
