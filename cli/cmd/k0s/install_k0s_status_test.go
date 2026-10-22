// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"context"
	"errors"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Single-node k0s readiness", func() {
	var readiness *installer.MockK0sReadinessClient

	BeforeEach(func() {
		readiness = installer.NewMockK0sReadinessClient(GinkgoT())
	})

	It("accepts a controller connected to the API", func() {
		ctx := context.Background()
		readiness.EXPECT().Status(ctx).Return([]byte(`{"pid":123,"role":"controller","workerToAPIConnectionStatus":{"success":true}}`), nil).Once()
		Expect(waitForK0sReady(ctx, time.Second, readiness)).To(Succeed())
	})

	It("reports the last status when canceled before API readiness", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		readiness.EXPECT().Status(ctx).Return([]byte(`{"pid":123,"role":"controller","workerToAPIConnectionStatus":{"success":false}}`), nil).Once()
		err := waitForK0sReady(ctx, time.Second, readiness)
		Expect(err).To(MatchError(ContainSubstring("apiReady=false")))
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})

	It("reports a status command failure when canceled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		readiness.EXPECT().Status(ctx).Return([]byte("k0s is not running"), errors.New("exit status 1")).Once()
		err := waitForK0sReady(ctx, time.Second, readiness)
		Expect(err).To(MatchError(ContainSubstring("k0s is not running")))
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})

	It("returns a registered node", func() {
		ctx := context.Background()
		readiness.EXPECT().Nodes(ctx).Return([]byte("node/first-node\n"), nil).Once()
		node, err := waitForK0sNode(ctx, time.Second, readiness)
		Expect(err).NotTo(HaveOccurred())
		Expect(node).To(Equal("node/first-node"))
	})

	It("reports missing node registration when canceled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		readiness.EXPECT().Nodes(ctx).Return([]byte("No resources found"), nil).Once()
		node, err := waitForK0sNode(ctx, time.Second, readiness)
		Expect(node).To(BeEmpty())
		Expect(err).To(MatchError(ContainSubstring("No resources found")))
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})

	It("reports a node query failure when canceled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		readiness.EXPECT().Nodes(ctx).Return([]byte("connection refused"), errors.New("exit status 1")).Once()
		_, err := waitForK0sNode(ctx, time.Second, readiness)
		Expect(err).To(MatchError(ContainSubstring("connection refused")))
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
})
