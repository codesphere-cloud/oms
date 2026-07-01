// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package docker_test

import (
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/internal/installer/docker"
	"github.com/codesphere-cloud/oms/internal/installer/node"
)

// isAddRepoCommand matches the multi-line command that adds Docker's apt
// repository, which is impractical to reproduce verbatim in test expectations.
func isAddRepoCommand(cmd string) bool {
	return strings.Contains(cmd, "download.docker.com/linux/ubuntu/gpg")
}

func fakeNode(name string, commandRunner node.NodeClient) *node.Node {
	return &node.Node{
		Name:       name,
		ExternalIP: "1.2.3.4",
		InternalIP: "10.0.0.1",

		NodeClient: commandRunner,
	}
}

var _ = Describe("Docker", func() {
	var (
		manager    docker.DockerManager
		nodeClient *node.MockNodeClient
		remoteNode *node.Node
	)

	BeforeEach(func() {
		nodeClient = node.NewMockNodeClient(GinkgoT())
		remoteNode = fakeNode("docker-host", nodeClient)

		manager = docker.New("ubuntu", remoteNode)
	})

	Describe("New", func() {
		It("creates a new DockerInstaller", func() {
			Expect(manager).ToNot(BeNil())
		})
	})

	Describe("IsInstalled", func() {
		It("returns true when the docker binary is available", func() {
			nodeClient.EXPECT().RunCommand(remoteNode, "ubuntu", "command -v docker").Return(nil)

			Expect(manager.IsInstalled()).To(BeTrue())
		})

		It("returns false when the docker binary is not available", func() {
			nodeClient.EXPECT().RunCommand(remoteNode, "ubuntu", "command -v docker").Return(errors.New("not found"))

			Expect(manager.IsInstalled()).To(BeFalse())
		})
	})

	Describe("InstallDocker", func() {
		It("fails when removing conflicting packages fails", func() {
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true").
				Return(errors.New("ssh error"))

			err := manager.InstallWithApt()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to remove conflicting docker packages"))
		})

		It("fails when installing apt prerequisites fails", func() {
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get update -qq").
				Return(errors.New("apt error"))

			err := manager.InstallWithApt()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install docker apt prequisites"))
		})

		It("fails when adding the docker repository fails", func() {
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get update -qq").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq ca-certificates curl").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", mock.MatchedBy(isAddRepoCommand)).
				Return(errors.New("repo error")).
				Once()

			err := manager.InstallWithApt()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to add docker apt repository"))
		})

		It("fails when installing docker packages fails", func() {
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get update -qq").
				Return(nil).
				Twice()
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq ca-certificates curl").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", mock.MatchedBy(isAddRepoCommand)).
				Return(nil).
				Once()
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin").
				Return(errors.New("install error"))

			err := manager.InstallWithApt()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install docker packages"))
		})

		It("fails when starting the docker daemon fails", func() {
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get update -qq").
				Return(nil).
				Twice()
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq ca-certificates curl").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", mock.MatchedBy(isAddRepoCommand)).
				Return(nil).
				Once()
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "systemctl start docker").
				Return(errors.New("daemon error"))

			err := manager.InstallWithApt()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to start docker daemon"))
		})

		It("succeeds when all steps succeed", func() {
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get update -qq").
				Return(nil).
				Twice()
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq ca-certificates curl").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", mock.MatchedBy(isAddRepoCommand)).
				Return(nil).
				Once()
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "systemctl start docker").
				Return(nil)
			nodeClient.EXPECT().
				RunCommand(remoteNode, "ubuntu", "systemctl enable docker").
				Return(nil)

			err := manager.InstallWithApt()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
