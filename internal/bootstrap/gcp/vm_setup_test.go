// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"

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

var _ = Describe("VM Setup", func() {
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

		csEnv = NewTestCodesphereEnvironment(nodeClient)
	})

	Describe("EnsureRootLoginEnabled", func() {
		Context("When WaitReady times out", func() {
			It("fails", func() {
				nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(fmt.Errorf("TIMEOUT!"))

				err := bs.EnsureRootLoginEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out waiting for SSH service"))
			})
		})
		Context("When WaitReady succeeds", func() {
			BeforeEach(func() {
				nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil)
			})
			Describe("Valid EnsureRootLoginEnabled", func() {
				It("enables root login on all nodes", func() {
					nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(nil)

					// Setup nodes

					err := bs.EnsureRootLoginEnabled()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			It("fails when EnableRootLogin fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(fmt.Errorf("ouch"))

				err := bs.EnsureRootLoginEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to enable root login"))
			})
		})
	})

	Describe("EnsureJumpboxConfigured", func() {
		Describe("Valid EnsureJumpboxConfigured", func() {
			It("configures jumpbox", func() {
				// Setup jumpbox node requires some commands to run
				nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

				err := bs.EnsureJumpboxConfigured()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when ConfigureAcceptEnv fails", func() {
				// Setup jumpbox node requires some commands to run
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(fmt.Errorf("ouch")).Twice()

				err := bs.EnsureJumpboxConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure AcceptEnv"))
			})

			It("fails when InstallOms fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(nil)
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("outch"))

				err := bs.EnsureJumpboxConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install OMS"))
			})
		})
	})

	Describe("EnsureHostsConfigured", func() {
		Describe("Valid EnsureHostsConfigured", func() {
			It("configures hosts", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil)

				err := bs.EnsureHostsConfigured()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when ConfigureInotifyWatches fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("ouch"))

				err := bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure inotify watches"))
			})

			It("fails when ConfigureMemoryMap fails", func() {
				mock.InOrder(
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil).Times(1),                // for inotify
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("ouch")).Times(2), // for memory map
				)

				err := bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure memory map"))
			})
		})
	})

	Describe("GenerateK0sConfigScript", func() {
		Describe("Valid GenerateK0sConfigScript", func() {
			It("generates script", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
				nodeClient.EXPECT().CopyFile(bs.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "chmod +x /root/configure-k0s.sh").Return(nil)

				err := bs.GenerateK0sConfigScript()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when WriteFile fails", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(fmt.Errorf("write error"))

				err := bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write configure-k0s.sh"))
			})

			It("fails when CopyFile fails", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
				nodeClient.EXPECT().CopyFile(mock.Anything, "configure-k0s.sh", "/root/configure-k0s.sh").Return(fmt.Errorf("copy error"))

				err := bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy configure-k0s.sh to control plane node"))
			})

			It("fails when RunSSHCommand chmod fails", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)

				nodeClient.EXPECT().CopyFile(bs.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "chmod +x /root/configure-k0s.sh").Return(fmt.Errorf("chmod error"))

				err := bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to make configure-k0s.sh executable"))
			})
		})
	})
})
