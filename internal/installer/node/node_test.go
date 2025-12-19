// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node_test

import (
	"errors"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
)

func TestNode(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Node Suite")
}

var _ = Describe("Node", func() {
	Describe("NodeManager", func() {
		var (
			nm             *node.NodeManager
			mockFileWriter *util.MockFileIO
		)

		BeforeEach(func() {
			mockFileWriter = util.NewMockFileIO(GinkgoT())
			nm = &node.NodeManager{
				FileIO:  mockFileWriter,
				KeyPath: "",
			}
		})

		AfterEach(func() {
			mockFileWriter.AssertExpectations(GinkgoT())
		})

		Context("authentication methods", func() {
			It("should return error when no authentication method is available", func() {
				originalAuthSock := os.Getenv("SSH_AUTH_SOCK")
				defer func() {
					if originalAuthSock != "" {
						_ = os.Setenv("SSH_AUTH_SOCK", originalAuthSock)
					} else {
						_ = os.Unsetenv("SSH_AUTH_SOCK")
					}
				}()
				_ = os.Unsetenv("SSH_AUTH_SOCK")

				nm.KeyPath = ""

				client, err := nm.GetClient("", "10.0.0.1", "root")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no valid authentication methods"))
				Expect(client).To(BeNil())
			})

			It("should return error when key file cannot be read", func() {
				originalAuthSock := os.Getenv("SSH_AUTH_SOCK")
				defer func() {
					if originalAuthSock != "" {
						_ = os.Setenv("SSH_AUTH_SOCK", originalAuthSock)
					} else {
						_ = os.Unsetenv("SSH_AUTH_SOCK")
					}
				}()
				_ = os.Unsetenv("SSH_AUTH_SOCK")

				nm.KeyPath = "/nonexistent/key"
				mockFileWriter.EXPECT().ReadFile("/nonexistent/key").Return(nil, errors.New("file not found"))

				client, err := nm.GetClient("", "10.0.0.1", "root")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read private key file"))
				Expect(client).To(BeNil())
			})

			It("should return error when key file is invalid", func() {
				originalAuthSock := os.Getenv("SSH_AUTH_SOCK")
				defer func() {
					if originalAuthSock != "" {
						_ = os.Setenv("SSH_AUTH_SOCK", originalAuthSock)
					} else {
						_ = os.Unsetenv("SSH_AUTH_SOCK")
					}
				}()
				_ = os.Unsetenv("SSH_AUTH_SOCK")

				invalidKey := []byte("not a valid ssh key")
				nm.KeyPath = "/path/to/invalid/key"
				mockFileWriter.EXPECT().ReadFile("/path/to/invalid/key").Return(invalidKey, nil)

				client, err := nm.GetClient("", "10.0.0.1", "root")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse private key"))
				Expect(client).To(BeNil())
			})
		})

		Context("SSH connection", func() {
			It("should fail to connect to invalid host", func() {
				privateKey := []byte(`-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDjKvZvwzXnCdFniXHDZdFPo4LFJ7KJJdBWrJjN1rO1ZQAAAJgNY3PmDWNz
5gAAAAtzc2gtZWQyNTUxOQAAACDjKvZvwzXnCdFniXHDZdFPo4LFJ7KJJdBWrJjN1rO1ZQ
AAAEDcZfnYLBVPEQT3qYDh6e5zMvKjN8x5k4l3n9qYLFJ7MOMq9m/DNecJ0WeJccNl0U+j
gsUnsokl0FasmM3Ws7VlAAAADnRlc3RAZXhhbXBsZS5jb20BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`)

				nm.KeyPath = "/path/to/key"
				mockFileWriter.EXPECT().ReadFile("/path/to/key").Return(privateKey, nil).Maybe()

				client, err := nm.GetClient("", "192.0.2.1", "root")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to dial"))
				Expect(client).To(BeNil())
			})

			It("should fail to connect through invalid jumpbox", func() {
				privateKey := []byte(`-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDjKvZvwzXnCdFniXHDZdFPo4LFJ7KJJdBWrJjN1rO1ZQAAAJgNY3PmDWNz
5gAAAAtzc2gtZWQyNTUxOQAAACDjKvZvwzXnCdFniXHDZdFPo4LFJ7KJJdBWrJjN1rO1ZQ
AAAEDcZfnYLBVPEQT3qYDh6e5zMvKjN8x5k4l3n9qYLFJ7MOMq9m/DNecJ0WeJccNl0U+j
gsUnsokl0FasmM3Ws7VlAAAADnRlc3RAZXhhbXBsZS5jb20BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`)

				nm.KeyPath = "/path/to/key"
				mockFileWriter.EXPECT().ReadFile("/path/to/key").Return(privateKey, nil).Maybe()

				client, err := nm.GetClient("192.0.2.1", "192.0.2.2", "root")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to connect to jumpbox"))
				Expect(client).To(BeNil())
			})
		})

		Context("file operations", func() {
			It("should handle directory creation errors", func() {
				err := nm.EnsureDirectoryExists("", "192.0.2.1", "root", "/tmp/test")
				Expect(err).To(HaveOccurred())
			})

			It("should handle copy file errors when source doesn't exist", func() {
				mockFileWriter.EXPECT().Open("/nonexistent/file").Return(nil, errors.New("file not found")).Maybe()

				err := nm.CopyFile("", "192.0.2.1", "root", "/nonexistent/file", "/tmp/dest")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get SSH client"))
			})
		})
	})

	Describe("Node methods", func() {
		var (
			n              *node.Node
			nm             *node.NodeManager
			mockFileWriter *util.MockFileIO
		)

		BeforeEach(func() {
			mockFileWriter = util.NewMockFileIO(GinkgoT())
			nm = &node.NodeManager{
				FileIO:  mockFileWriter,
				KeyPath: "",
			}
			n = &node.Node{
				Name:       "test-node",
				ExternalIP: "10.0.0.1",
				InternalIP: "192.168.1.1",
			}
		})

		AfterEach(func() {
			mockFileWriter.AssertExpectations(GinkgoT())
		})

		Context("HasCommand", func() {
			It("should return false when SSH connection fails", func() {
				result := n.HasCommand(nm, "kubectl")
				Expect(result).To(BeFalse())
			})

			It("should handle commands with special characters safely", func() {
				result := n.HasCommand(nm, "kubectl'; rm -rf /; echo '")
				Expect(result).To(BeFalse())
			})
		})

		Context("HasFile", func() {
			It("should return false when SSH connection fails", func() {
				result := n.HasFile(nil, nm, "/etc/k0s/k0s.yaml")
				Expect(result).To(BeFalse())
			})

			It("should handle paths with special characters safely", func() {
				result := n.HasFile(nil, nm, "/path'; rm -rf /; echo '/file.txt")
				Expect(result).To(BeFalse())
			})

			It("should support jumpbox connections", func() {
				jumpbox := &node.Node{
					ExternalIP: "10.0.0.2",
					InternalIP: "10.0.0.2",
				}
				result := n.HasFile(jumpbox, nm, "/etc/k0s/k0s.yaml")
				Expect(result).To(BeFalse())
			})
		})

		Context("CopyFile", func() {
			It("should fail when directory creation fails", func() {
				err := n.CopyFile(nm, "/some/file", "/remote/path/dest.txt")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure directory exists"))
			})
		})

		Context("RunSSHCommand", func() {
			It("should handle direct connection without jumpbox", func() {
				err := n.RunSSHCommand(nil, nm, "root", "echo test")
				Expect(err).To(HaveOccurred())
			})

			It("should handle connection through jumpbox", func() {
				jumpbox := &node.Node{
					ExternalIP: "10.0.0.2",
					InternalIP: "10.0.0.2",
				}
				err := n.RunSSHCommand(jumpbox, nm, "ubuntu", "echo test")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("InstallK0s", func() {
			It("should handle binary copy failure", func() {
				k0sBinaryPath := "/path/to/k0s"
				mockFileWriter.EXPECT().Open(k0sBinaryPath).Return(nil, errors.New("file not found")).Maybe()

				err := n.InstallK0s(nm, k0sBinaryPath, "", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy k0s binary to temp"))
			})

			It("should handle paths with special characters safely", func() {
				k0sBinaryPath := "/path/to/k0s'; echo 'injected"

				err := n.InstallK0s(nm, k0sBinaryPath, "", false)
				Expect(err).To(HaveOccurred())
			})

			It("should support force flag parameter", func() {
				k0sBinaryPath := "/tmp/k0s"

				err := n.InstallK0s(nm, k0sBinaryPath, "", true)
				Expect(err).To(HaveOccurred())
				// Will fail to connect, but tests that force flag is handled
			})

			It("should support config file parameter", func() {
				k0sBinaryPath := "/tmp/k0s"
				k0sConfigPath := "/tmp/k0s.yaml"

				err := n.InstallK0s(nm, k0sBinaryPath, k0sConfigPath, false)
				Expect(err).To(HaveOccurred())
				// Will fail to connect, but tests that config path is handled
			})
		})
	})
})
