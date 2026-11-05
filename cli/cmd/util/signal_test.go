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
	It("derives the command context from the parent context", func() {
		parent, cancel := context.WithCancel(context.Background())
		cmd := &cobra.Command{}
		cmd.SetContext(parent)

		var runCtx context.Context

		run := util.CancelOnSignal(func(cmd *cobra.Command, _ []string) error {
			runCtx = cmd.Context()

			cancel()

			return runCtx.Err()
		})

		Expect(run(cmd, nil)).To(MatchError(context.Canceled))
		Expect(runCtx).NotTo(BeIdenticalTo(parent))
	})

	It("restores the parent context so the command can run again", func() {
		parent := context.Background()
		cmd := &cobra.Command{}
		cmd.SetContext(parent)

		run := util.CancelOnSignal(func(cmd *cobra.Command, _ []string) error {
			return cmd.Context().Err()
		})

		Expect(run(cmd, nil)).To(Succeed())
		Expect(cmd.Context()).To(BeIdenticalTo(parent))
		Expect(run(cmd, nil)).To(Succeed())
	})

	It("returns the error of the wrapped command", func() {
		runErr := errors.New("install failed")
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())

		run := util.CancelOnSignal(func(_ *cobra.Command, _ []string) error {
			return runErr
		})

		Expect(run(cmd, nil)).To(MatchError(runErr))
	})
})
