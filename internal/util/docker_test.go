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
		It("updates a simple FROM statement", func() {
			dockerfile := `FROM ubuntu:20.04
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM ubuntu:22.04
RUN apt-get update
WORKDIR /app`))
		})

		It("updates FROM statement with platform flag", func() {
			dockerfile := `FROM --platform=linux/amd64 ubuntu:20.04
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM ubuntu:22.04
RUN apt-get update
WORKDIR /app`))
		})

		It("updates FROM statement with tabs", func() {
			dockerfile := "\tFROM ubuntu:20.04\nRUN apt-get update\nWORKDIR /app"

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("\tFROM ubuntu:22.04\nRUN apt-get update\nWORKDIR /app"))
		})

		It("handles case-insensitive FROM statement", func() {
			dockerfile := `from ubuntu:20.04
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`from ubuntu:22.04
RUN apt-get update
WORKDIR /app`))
		})

		It("updates only the first FROM statement in multi-stage builds", func() {
			dockerfile := `FROM ubuntu:20.04 as builder
RUN apt-get update
FROM alpine:3.14
COPY --from=builder /app /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM ubuntu:22.04 as builder
RUN apt-get update
FROM alpine:3.14
COPY --from=builder /app /app`))
		})

		It("handles FROM statement with AS alias", func() {
			dockerfile := `FROM ubuntu:20.04 AS builder
RUN apt-get update
WORKDIR /app`

			result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`FROM ubuntu:22.04 AS builder
RUN apt-get update
WORKDIR /app`))
		})

		Context("edge cases", func() {
			It("handles ARG statements before FROM", func() {
				dockerfile := `ARG BASE_IMAGE=ubuntu:20.04
ARG VERSION=latest
FROM ${BASE_IMAGE}
RUN apt-get update
WORKDIR /app`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`ARG BASE_IMAGE=ubuntu:20.04
ARG VERSION=latest
FROM ubuntu:22.04
RUN apt-get update
WORKDIR /app`))
			})

			It("handles comments before FROM", func() {
				dockerfile := `# This is a comment
# Another comment about the base image
FROM ubuntu:20.04
RUN apt-get update
WORKDIR /app`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`# This is a comment
# Another comment about the base image
FROM ubuntu:22.04
RUN apt-get update
WORKDIR /app`))
			})

			It("handles empty lines before FROM", func() {
				dockerfile := `

ARG BASE_IMAGE=ubuntu:20.04

FROM ${BASE_IMAGE}
RUN apt-get update`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`

ARG BASE_IMAGE=ubuntu:20.04

FROM ubuntu:22.04
RUN apt-get update`))
			})
		})

		Context("error cases", func() {
			It("returns error when no FROM statement found", func() {
				dockerfile := `RUN apt-get update
WORKDIR /app
COPY . .`

				_, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no FROM statement found in dockerfile"))
			})

			It("returns error when dockerfile is empty", func() {
				dockerfile := ""

				_, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no FROM statement found in dockerfile"))
			})
		})

		Context("regression tests", func() {
			It("handles FROM with multiple spaces", func() {
				dockerfile := `FROM    ubuntu:20.04
RUN apt-get update`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`FROM    ubuntu:22.04
RUN apt-get update`))
			})

			It("handles FROM at end of file without newline", func() {
				dockerfile := `FROM ubuntu:20.04`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`FROM ubuntu:22.04`))
			})

			It("handles FROM with trailing spaces", func() {
				dockerfile := `FROM ubuntu:20.04   
RUN apt-get update`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`FROM ubuntu:22.04
RUN apt-get update`))
			})

			It("handles FROM with leading spaces", func() {
				dockerfile := `    FROM ubuntu:20.04
RUN apt-get update
WORKDIR /app`

				result, err := dockerfileManager.UpdateFromStatement(strings.NewReader(dockerfile), "ubuntu:22.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(`    FROM ubuntu:22.04
RUN apt-get update
WORKDIR /app`))
			})
		})
	})
})
