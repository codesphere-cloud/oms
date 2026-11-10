// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/system"
)

var _ = Describe("CommandContext", func() {
	It("interrupts instead of killing on cancellation and bounds the wait with the grace period", func() {
		cmd := system.CommandContext(context.Background(), "node", "installer.js")

		Expect(cmd.Args).To(Equal([]string{"node", "installer.js"}))
		Expect(cmd.Cancel).NotTo(BeNil())
		Expect(cmd.WaitDelay).To(Equal(system.InterruptGracePeriod))
	})
})
