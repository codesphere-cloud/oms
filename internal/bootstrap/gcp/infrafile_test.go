// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"os"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Infrafile", func() {
	var (
		nodeClient *node.MockNodeClient
		csEnv      *gcp.CodesphereEnvironment
		ctx        context.Context
		e          env.Env

		icg              *installer.MockInstallConfigManager
		gc               *gcp.MockGCPClientManager
		fw               *util.MockFileIO
		stlog            *bootstrap.StepLogger
		mockPortalClient *portal.MockPortal
		mockGitHubClient *github.MockGitHubClient

		bs *gcp.GCPBootstrapper
	)

	JustBeforeEach(func() {
		var err error
		bs, err = gcp.NewGCPBootstrapper(
			ctx,
			e,
			stlog,
			csEnv,
			icg,
			gc,
			fw,
			nodeClient,
			mockPortalClient,
			util.NewFakeTime(),
			mockGitHubClient,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		nodeClient = node.NewMockNodeClient(GinkgoT())
		ctx = context.Background()
		e = env.NewEnv()
		icg = installer.NewMockInstallConfigManager(GinkgoT())
		gc = gcp.NewMockGCPClientManager(GinkgoT())
		fw = util.NewMockFileIO(GinkgoT())
		mockPortalClient = portal.NewMockPortal(GinkgoT())
		mockGitHubClient = github.NewMockGitHubClient(GinkgoT())
		stlog = bootstrap.NewStepLogger(false)

		csEnv = &gcp.CodesphereEnvironment{
			ProjectName: "test-project",
		}
	})

	const (
		infraFilePath = "/test/workdir/gcp-infra.json"
	)

	Describe("GetInfraFilePath", func() {
		AfterEach(func() {
			err := os.Unsetenv("OMS_WORKDIR")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns the default path for the infra file when env var is not set", func() {
			actualPath := gcp.GetInfraFilePath()

			Expect(actualPath).To(Equal("./oms-workdir/gcp-infra.json"))
		})

		It("returns the correct path for the infra file based on the OMS_WORKDIR env var", func() {
			err := os.Setenv("OMS_WORKDIR", "/test/workdir")
			Expect(err).ToNot(HaveOccurred())

			actualPath := gcp.GetInfraFilePath()

			Expect(actualPath).To(Equal("/test/workdir/gcp-infra.json"))
		})
	})

	Describe("LoadInfraFile", func() {
		It("returns exists=false when the infra file does not exist", func() {
			fw.EXPECT().Exists(infraFilePath).Return(false)

			env, exists, err := gcp.LoadInfraFile(fw, infraFilePath)

			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
			Expect(env).To(Equal(gcp.CodesphereEnvironment{}))
		})

		It("returns an error when the infra file cannot be read", func() {
			fw.EXPECT().Exists(infraFilePath).Return(true)
			fw.EXPECT().ReadFile(infraFilePath).Return(nil, os.ErrPermission)

			env, exists, err := gcp.LoadInfraFile(fw, infraFilePath)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read gcp infra file"))
			Expect(exists).To(BeTrue())
			Expect(env).To(Equal(gcp.CodesphereEnvironment{}))
		})

		It("returns an error when the infra file contains invalid JSON", func() {
			fw.EXPECT().Exists(infraFilePath).Return(true)
			fw.EXPECT().ReadFile(infraFilePath).Return([]byte("invalid json"), nil)

			env, exists, err := gcp.LoadInfraFile(fw, infraFilePath)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmarshal gcp infra file"))
			Expect(exists).To(BeTrue())
			Expect(env).To(Equal(gcp.CodesphereEnvironment{}))
		})

		It("successfully loads and parses the infra file", func() {
			fw.EXPECT().Exists(infraFilePath).Return(true)
			fw.EXPECT().ReadFile(infraFilePath).Return([]byte(`{"project_id":"test-project","region":"us-central1"}`), nil)

			env, exists, err := gcp.LoadInfraFile(fw, infraFilePath)

			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
			Expect(env).To(Equal(gcp.CodesphereEnvironment{
				ProjectID: "test-project",
				Region:    "us-central1",
			}))
		})
	})

	Describe("WriteInfraFile", func() {
		It("returns an error when the workdir cannot be created", func() {
			fw.EXPECT().MkdirAll(mock.Anything, os.FileMode(0755)).Return(os.ErrPermission)

			err := bs.WriteInfraFile()
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the infra file cannot be written", func() {
			fw.EXPECT().MkdirAll(mock.Anything, os.FileMode(0755)).Return(nil)
			fw.EXPECT().WriteFile(mock.Anything, mock.Anything, os.FileMode(0644)).Return(os.ErrPermission)

			err := bs.WriteInfraFile()
			Expect(err).To(HaveOccurred())
		})

		It("successfully writes the infra file", func() {
			fw.EXPECT().MkdirAll(mock.Anything, os.FileMode(0755)).Return(nil)
			fw.EXPECT().WriteFile(mock.Anything, mock.Anything, os.FileMode(0644)).Return(nil)

			err := bs.WriteInfraFile()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
