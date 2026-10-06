// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// writeKeyPair writes a new ed25519 key pair to dir and returns the private key path and the key.
func writeKeyPair(dir string, name string) (string, ed25519.PrivateKey) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	block, err := ssh.MarshalPrivateKey(priv, "")
	Expect(err).NotTo(HaveOccurred())
	signer, err := ssh.NewSignerFromKey(priv)
	Expect(err).NotTo(HaveOccurred())

	keyPath := filepath.Join(dir, name)
	Expect(os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600)).To(Succeed())
	Expect(os.WriteFile(keyPath+".pub", ssh.MarshalAuthorizedKey(signer.PublicKey()), 0644)).To(Succeed())

	return keyPath, priv
}

func listedKeys(a agent.Agent) []string {
	keys, err := a.List()
	Expect(err).NotTo(HaveOccurred())

	blobs := make([]string, 0, len(keys))
	for _, k := range keys {
		blobs = append(blobs, string(k.Marshal()))
	}

	return blobs
}

func publicBlob(priv ed25519.PrivateKey) string {
	signer, err := ssh.NewSignerFromKey(priv)
	Expect(err).NotTo(HaveOccurred())

	return string(signer.PublicKey().Marshal())
}

var _ = Describe("forwardedAgent", func() {
	var (
		dir     string
		keyPath string
		deploy  ed25519.PrivateKey
		n       *Node
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		keyPath, deploy = writeKeyPair(dir, "deploy")
		n = &Node{FileIO: util.NewFilesystemWriter(), KeyPath: keyPath}
	})

	It("forwards nothing without a local agent or key path", func() {
		n.KeyPath = ""

		forwarded, err := n.forwardedAgent(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(forwarded).To(BeNil())
	})

	It("forwards the local agent as-is when it holds the deploy key", func() {
		local := agent.NewKeyring().(agent.ExtendedAgent)
		Expect(local.Add(agent.AddedKey{PrivateKey: deploy})).To(Succeed())

		forwarded, err := n.forwardedAgent(local)
		Expect(err).NotTo(HaveOccurred())
		Expect(forwarded).To(BeIdenticalTo(local))
	})

	It("forwards only the deploy key without a local agent", func() {
		forwarded, err := n.forwardedAgent(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(listedKeys(forwarded)).To(ConsistOf(publicBlob(deploy)))
	})

	It("adds the deploy key to a local agent that lacks it and signs with both keys", func() {
		_, other := writeKeyPair(dir, "other")
		local := agent.NewKeyring().(agent.ExtendedAgent)
		Expect(local.Add(agent.AddedKey{PrivateKey: other})).To(Succeed())

		forwarded, err := n.forwardedAgent(local)
		Expect(err).NotTo(HaveOccurred())
		Expect(listedKeys(forwarded)).To(ConsistOf(publicBlob(deploy), publicBlob(other)))

		signers, err := forwarded.Signers()
		Expect(err).NotTo(HaveOccurred())
		Expect(signers).To(HaveLen(2))

		data := []byte("challenge")
		for _, s := range signers {
			sig, err := forwarded.Sign(s.PublicKey(), data)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.PublicKey().Verify(data, sig)).To(Succeed())
		}
	})

	It("falls back to the local agent when the deploy key cannot be loaded", func() {
		n.KeyPath = filepath.Join(dir, "missing")
		local := agent.NewKeyring().(agent.ExtendedAgent)

		forwarded, err := n.forwardedAgent(local)
		Expect(err).NotTo(HaveOccurred())
		Expect(forwarded).To(BeIdenticalTo(local))
	})

	It("fails without a local agent when the deploy key cannot be loaded", func() {
		n.KeyPath = filepath.Join(dir, "missing")

		_, err := n.forwardedAgent(nil)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("dialLocalAgent", func() {
	It("returns no agent without SSH_AUTH_SOCK", func() {
		GinkgoT().Setenv("SSH_AUTH_SOCK", "")

		localAgent, closeAgent, err := dialLocalAgent()
		Expect(err).NotTo(HaveOccurred())
		Expect(localAgent).To(BeNil())
		closeAgent()
	})

	It("fails when the agent socket cannot be reached", func() {
		GinkgoT().Setenv("SSH_AUTH_SOCK", filepath.Join(GinkgoT().TempDir(), "missing.sock"))

		localAgent, closeAgent, err := dialLocalAgent()
		Expect(err).To(HaveOccurred())
		Expect(localAgent).To(BeNil())
		closeAgent()
	})

	It("connects to the agent and closes the connection", func() {
		// Unix socket paths are limited to ~104 bytes on macOS, so keep the path short.
		dir, err := os.MkdirTemp("", "agent")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, dir)

		socketPath := filepath.Join(dir, "a.sock")
		listener, err := net.Listen("unix", socketPath)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(listener.Close)

		served := make(chan error, 1)

		go func() {
			conn, err := listener.Accept()
			if err != nil {
				served <- err
				return
			}
			// ServeAgent returns once the client closes its end of the connection.
			_ = agent.ServeAgent(agent.NewKeyring(), conn)
			served <- conn.Close()
		}()

		GinkgoT().Setenv("SSH_AUTH_SOCK", socketPath)

		localAgent, closeAgent, err := dialLocalAgent()
		Expect(err).NotTo(HaveOccurred())
		Expect(localAgent).NotTo(BeNil())
		Expect(localAgent.List()).To(BeEmpty())

		closeAgent()
		Eventually(served).Should(Receive(Succeed()))
	})
})
