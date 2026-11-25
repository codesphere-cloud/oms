// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"

	"github.com/codesphere-cloud/oms/internal/installer/node"
)

var _ = Describe("WaitForSession", func() {
	var session *node.MockRemoteSession

	BeforeEach(func() {
		session = node.NewMockRemoteSession(GinkgoT())
	})

	It("returns the result of the remote command", func() {
		session.EXPECT().Wait().Return(errors.New("exit status 1")).Once()

		err := node.WaitForSession(context.Background(), session)

		Expect(err).To(MatchError("exit status 1"))
	})

	It("interrupts the remote command and stops waiting once the context is cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})

		DeferCleanup(func() { close(release) })

		session.EXPECT().Wait().RunAndReturn(func() error {
			cancel()
			<-release

			return errors.New("session closed")
		}).Once()
		session.EXPECT().Signal(ssh.SIGINT).Return(nil).Once()
		session.EXPECT().Close().Return(nil).Once()

		err := node.WaitForSession(ctx, session)

		Expect(err).To(MatchError(context.Canceled))
	})
})
