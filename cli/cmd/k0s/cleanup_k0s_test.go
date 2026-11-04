// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
)

var _ = Describe("CleanupK0sCmd", func() {
	var (
		c              k0s.CleanupK0sCmd
		mockNodeClient *node.MockNodeClient
		nodes          []*node.Node
	)

	newNode := func(ip string) *node.Node {
		return &node.Node{Name: ip, ExternalIP: ip, NodeClient: mockNodeClient}
	}

	BeforeEach(func() {
		mockNodeClient = node.NewMockNodeClient(GinkgoT())
		nodes = []*node.Node{newNode("10.0.0.1"), newNode("10.0.0.2")}
		c = k0s.CleanupK0sCmd{
			Opts:          k0s.CleanupK0sOpts{GlobalOptions: &util.GlobalOptions{}, Force: true},
			ConfirmReader: strings.NewReader(""),
		}
	})

	Context("RunE method", func() {
		var tempDir string

		BeforeEach(func() {
			var err error

			tempDir, err = os.MkdirTemp("", "cleanup-k0s-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_ = os.RemoveAll(tempDir)
		})

		It("fails for an install-config with external Kubernetes", func() {
			configData, err := yaml.Marshal(&files.RootConfig{
				Kubernetes: files.KubernetesConfig{ManagedByCodesphere: false},
			})
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = filepath.Join(tempDir, "install-config.yaml")
			Expect(os.WriteFile(c.Opts.InstallConfig, configData, 0644)).To(Succeed())

			err = c.RunE(nil, nil)
			Expect(err).To(MatchError(ContainSubstring("only supported for Codesphere-managed Kubernetes")))
		})

		It("fails when the install-config does not exist", func() {
			c.Opts.InstallConfig = filepath.Join(tempDir, "missing.yaml")

			err := c.RunE(nil, nil)
			Expect(err).To(MatchError(ContainSubstring("failed to load install-config")))
		})
	})

	Context("CleanupK0s method", func() {
		It("cleans up and reboots every node", func() {
			for _, n := range nodes {
				mockNodeClient.EXPECT().RunCommand(n, "root", installer.K0sCleanupScript()).Return(nil).Once()
				mockNodeClient.EXPECT().RunCommand(n, "root", installer.K0sRebootCommand).Return(nil).Once()
			}

			Expect(c.CleanupK0s(nodes)).To(Succeed())
		})

		It("does not reboot with --skip-reboot", func() {
			c.Opts.SkipReboot = true

			for _, n := range nodes {
				mockNodeClient.EXPECT().RunCommand(n, "root", installer.K0sCleanupScript()).Return(nil).Once()
			}

			Expect(c.CleanupK0s(nodes)).To(Succeed())
		})

		It("continues with the remaining nodes and skips the reboot of failed ones", func() {
			mockNodeClient.EXPECT().RunCommand(nodes[0], "root", installer.K0sCleanupScript()).Return(errors.New("connection refused")).Once()
			mockNodeClient.EXPECT().RunCommand(nodes[1], "root", installer.K0sCleanupScript()).Return(nil).Once()
			mockNodeClient.EXPECT().RunCommand(nodes[1], "root", installer.K0sRebootCommand).Return(nil).Once()

			err := c.CleanupK0s(nodes)
			Expect(err).To(MatchError(ContainSubstring("failed to clean up k0s on 10.0.0.1")))
			Expect(err).To(MatchError(ContainSubstring("connection refused")))
		})

		It("reports reboot failures", func() {
			c.Opts.SkipReboot = false

			mockNodeClient.EXPECT().RunCommand(nodes[0], "root", installer.K0sCleanupScript()).Return(nil).Once()
			mockNodeClient.EXPECT().RunCommand(nodes[0], "root", installer.K0sRebootCommand).Return(errors.New("boom")).Once()

			err := c.CleanupK0s(nodes[:1])
			Expect(err).To(MatchError(ContainSubstring("failed to reboot 10.0.0.1")))
		})

		It("fails without nodes", func() {
			err := c.CleanupK0s(nil)
			Expect(err).To(MatchError(ContainSubstring("does not contain any Kubernetes nodes")))
		})

		It("runs after the cleanup is confirmed", func() {
			c.Opts.Force = false
			c.Opts.SkipReboot = true
			c.ConfirmReader = strings.NewReader("yes\n")

			for _, n := range nodes {
				mockNodeClient.EXPECT().RunCommand(n, "root", installer.K0sCleanupScript()).Return(nil).Once()
			}

			Expect(c.CleanupK0s(nodes)).To(Succeed())
		})

		It("aborts without touching any node when the cleanup is not confirmed", func() {
			c.Opts.Force = false
			c.ConfirmReader = strings.NewReader("no\n")

			err := c.CleanupK0s(nodes)
			Expect(err).To(MatchError("cleanup not confirmed, aborting"))
		})
	})
})
