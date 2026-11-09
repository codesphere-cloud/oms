// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
)

var _ = Describe("CancelOnSignal", func() {
	It("derives the run context from the command context", func() {
		parent, cancel := context.WithCancel(context.Background())
		cmd := &cobra.Command{}
		cmd.SetContext(parent)

		var runCtx context.Context

		runE := util.CancelOnSignal(func(ctx context.Context) error {
			runCtx = ctx

			cancel()

			return ctx.Err()
		})

		Expect(runE(cmd, nil)).To(MatchError(context.Canceled))
		Expect(runCtx).NotTo(BeIdenticalTo(parent))
	})

	It("leaves the command context untouched so the command can run again", func() {
		parent := context.Background()
		cmd := &cobra.Command{}
		cmd.SetContext(parent)

		runE := util.CancelOnSignal(func(ctx context.Context) error {
			return ctx.Err()
		})

		Expect(runE(cmd, nil)).To(Succeed())
		Expect(cmd.Context()).To(BeIdenticalTo(parent))
		Expect(runE(cmd, nil)).To(Succeed())
	})

	It("returns the error of the wrapped run", func() {
		runErr := errors.New("install failed")
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())

		runE := util.CancelOnSignal(func(_ context.Context) error {
			return runErr
		})

		Expect(runE(cmd, nil)).To(MatchError(runErr))
	})
})
