// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
