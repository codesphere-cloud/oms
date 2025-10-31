// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"strings"

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
			dockerfile := `FROM workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("updates FROM statement with workspace-agent and platform flag", func() {
			dockerfile := `FROM --platform=linux/amd64 workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("handles case-insensitive FROM statement with workspace-agent", func() {
			dockerfile := `from workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`from workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("handles FROM with complex workspace-agent image names", func() {
			dockerfile := `FROM registry.example.com:5000/workspace-agent-24.04:codesphere-v1.0.0
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "new-registry.com/workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM new-registry.com/workspace-agent-24.04:codesphere-v1.0.1
RUN apt-get update
WORKDIR /app`))
		})

		It("updates the last FROM statement in multi-stage builds", func() {
			dockerfile := `FROM alpine:3.14
RUN apt-get update
FROM workspace-agent-24.04:20.04 as builder
COPY --from=0 /app /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM alpine:3.14
RUN apt-get update
FROM workspace-agent-24.04:codesphere-v1.0.1 as builder
COPY --from=0 /app /app`))
		})

		It("returns error when no FROM statement with workspace-agent found", func() {
			dockerfile := `FROM ubuntu:20.04
RUN apt-get update
WORKDIR /app`

			_, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "workspace-agent-24.04:codesphere-v1.0.1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no FROM statement with workspace-agent found in dockerfile"))
		})
	})
})
