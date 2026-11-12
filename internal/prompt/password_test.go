// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package prompt_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/term"

	"github.com/codesphere-cloud/oms/internal/prompt"
)

var _ = Describe("ReadPassword", func() {
	const fd = 0

	var (
		terminal *prompt.MockTerminal
		state    *term.State
	)

	BeforeEach(func() {
		terminal = prompt.NewMockTerminal(GinkgoT())
		state = &term.State{}
	})

	It("returns the entered password", func() {
		terminal.EXPECT().GetState(fd).Return(state, nil).Once()
		terminal.EXPECT().ReadPassword(fd).Return([]byte("secret"), nil).Once()

		password, err := prompt.ReadPassword(context.Background(), terminal, fd)

		Expect(err).NotTo(HaveOccurred())
		Expect(password).To(Equal([]byte("secret")))
	})

	It("restores the terminal and stops waiting when interrupted", func() {
		ctx, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})

		DeferCleanup(func() { close(release) })

		terminal.EXPECT().GetState(fd).Return(state, nil).Once()
		terminal.EXPECT().ReadPassword(fd).RunAndReturn(func(int) ([]byte, error) {
			cancel()
			<-release

			return nil, errors.New("read aborted")
		}).Once()
		terminal.EXPECT().Restore(fd, state).Return(nil).Once()

		_, err := prompt.ReadPassword(ctx, terminal, fd)

		Expect(err).To(MatchError(context.Canceled))
	})

	It("does not read when the command was interrupted before the prompt", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		terminal.EXPECT().GetState(fd).Return(state, nil).Once()

		_, err := prompt.ReadPassword(ctx, terminal, fd)

		Expect(err).To(MatchError(context.Canceled))
	})

	It("reports a terminal that cannot be restored", func() {
		ctx, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})

		DeferCleanup(func() { close(release) })

		terminal.EXPECT().GetState(fd).Return(state, nil).Once()
		terminal.EXPECT().ReadPassword(fd).RunAndReturn(func(int) ([]byte, error) {
			cancel()
			<-release

			return nil, errors.New("read aborted")
		}).Once()
		terminal.EXPECT().Restore(fd, state).Return(errors.New("not a terminal")).Once()

		_, err := prompt.ReadPassword(ctx, terminal, fd)

		Expect(err).To(MatchError(ContainSubstring("failed to restore terminal")))
		Expect(err).To(MatchError(context.Canceled))
	})

	It("fails when the terminal state cannot be saved", func() {
		terminal.EXPECT().GetState(fd).Return(nil, errors.New("not a terminal")).Once()

		_, err := prompt.ReadPassword(context.Background(), terminal, fd)

		Expect(err).To(MatchError(ContainSubstring("failed to save terminal state")))
	})
})
