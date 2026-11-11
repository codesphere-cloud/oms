// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"bufio"
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// blockingReader returns a reader that never yields input, like a terminal nobody types into.
func blockingReader() io.Reader {
	reader, writer := io.Pipe()

	DeferCleanup(func() { Expect(writer.Close()).To(Succeed()) })

	return reader
}

var _ = Describe("ContextReader", func() {
	It("returns the input of the underlying reader", func() {
		reader := bufio.NewReader(NewContextReader(context.Background(), strings.NewReader("yes\n")))

		line, err := reader.ReadString('\n')

		Expect(err).NotTo(HaveOccurred())
		Expect(line).To(Equal("yes\n"))
	})

	It("does not read once the context is cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := NewContextReader(ctx, strings.NewReader("yes\n")).Read(make([]byte, 8))

		Expect(err).To(MatchError(context.Canceled))
	})

	It("stops waiting for input when the context is cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		go cancel()

		_, err := NewContextReader(ctx, blockingReader()).Read(make([]byte, 8))

		Expect(err).To(MatchError(context.Canceled))
	})
})

var _ = Describe("interruptReader", func() {
	var (
		notifier *MockSignalNotifier
		exiter   *MockExiter
	)

	BeforeEach(func() {
		notifier = NewMockSignalNotifier(GinkgoT())
		exiter = NewMockExiter(GinkgoT())
	})

	It("returns the input when there is no interrupt", func() {
		notifier.EXPECT().NotifyContext(context.Background()).Return(context.WithCancel(context.Background())).Once()
		reader := &interruptReader{ctx: context.Background(), reader: strings.NewReader("yes\n"), notifier: notifier, exiter: exiter}

		line, err := bufio.NewReader(reader).ReadString('\n')

		Expect(err).NotTo(HaveOccurred())
		Expect(line).To(Equal("yes\n"))
	})

	It("ends the process with exit code 130 when interrupted while waiting for input", func() {
		interrupted, cancel := context.WithCancel(context.Background())
		cancel()
		notifier.EXPECT().NotifyContext(context.Background()).Return(interrupted, cancel).Once()
		exiter.EXPECT().Exit(130).Once()
		reader := &interruptReader{ctx: context.Background(), reader: blockingReader(), notifier: notifier, exiter: exiter}

		_, err := reader.Read(make([]byte, 8))

		Expect(err).To(MatchError(io.EOF))
	})

	It("ends the process when the command was interrupted before the prompt started", func() {
		commandCtx, interruptCommand := context.WithCancel(context.Background())
		interruptCommand()
		notifier.EXPECT().NotifyContext(commandCtx).RunAndReturn(func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		}).Once()
		exiter.EXPECT().Exit(130).Once()
		reader := &interruptReader{ctx: commandCtx, reader: blockingReader(), notifier: notifier, exiter: exiter}

		_, err := reader.Read(make([]byte, 8))

		Expect(err).To(MatchError(io.EOF))
	})

	It("ends the process when a prompt is interrupted", func() {
		interrupted, cancel := context.WithCancel(context.Background())
		cancel()
		notifier.EXPECT().NotifyContext(context.Background()).Return(interrupted, cancel).Once()
		exiter.EXPECT().Exit(130).Once()
		p := &StdinPrompter{
			reader:      bufio.NewReader(&interruptReader{ctx: context.Background(), reader: blockingReader(), notifier: notifier, exiter: exiter}),
			interactive: true,
		}

		Expect(p.Bool("Continue?", false)).To(BeFalse())
	})
})
