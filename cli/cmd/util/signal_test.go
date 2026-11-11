// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"bytes"
	"context"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
)

var _ = Describe("NotifySignalContext", func() {
	It("stays active until a signal arrives or stop is called", func() {
		ctx, stop := util.NotifySignalContext(context.Background())
		defer stop()

		Expect(ctx.Err()).NotTo(HaveOccurred())
	})

	It("is cancelled when its parent is cancelled", func() {
		parent, cancel := context.WithCancel(context.Background())

		ctx, stop := util.NotifySignalContext(parent)
		defer stop()

		cancel()

		Expect(ctx.Err()).To(MatchError(context.Canceled))
	})

	It("is cancelled by stop", func() {
		ctx, stop := util.NotifySignalContext(context.Background())

		stop()

		Expect(ctx.Err()).To(MatchError(context.Canceled))
	})
})

var _ = Describe("NewSignalContext", func() {
	var (
		source  *util.MockSignalSource
		out     *bytes.Buffer
		signals chan<- os.Signal
	)

	BeforeEach(func() {
		source = util.NewMockSignalSource(GinkgoT())
		out = &bytes.Buffer{}

		source.EXPECT().Notify(mock.Anything).Run(func(c chan<- os.Signal) {
			signals = c
		}).Once()
	})

	It("is cancelled on the first signal, restores default handling and says how to force", func() {
		source.EXPECT().Stop(mock.Anything).Return()

		ctx, stop := util.NewSignalContext(context.Background(), source, out)
		defer stop()

		signals <- os.Interrupt

		<-ctx.Done()

		Expect(ctx.Err()).To(MatchError(context.Canceled))
		Expect(out.String()).To(Equal("Interrupt received, stopping. Press Ctrl-C again to force.\n"))
	})

	It("prints nothing when stopped without a signal", func() {
		source.EXPECT().Stop(mock.Anything).Return().Once()

		ctx, stop := util.NewSignalContext(context.Background(), source, out)
		stop()

		Expect(ctx.Err()).To(MatchError(context.Canceled))
		Expect(out.String()).To(BeEmpty())
	})
})

var _ = Describe("ExitCode", func() {
	It("is 0 when the command succeeded", func() {
		Expect(util.ExitCode(context.Background(), nil)).To(Equal(0))
	})

	It("is 1 when the command failed on its own", func() {
		Expect(util.ExitCode(context.Background(), errors.New("boom"))).To(Equal(1))
	})

	It("is 130 when the command failed after an interrupt", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		Expect(util.ExitCode(ctx, errors.New("stopped waiting for input: context canceled"))).To(Equal(util.InterruptExitCode))
	})
})
