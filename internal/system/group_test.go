// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package system_test

import (
	"context"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/system"
)

var _ = Describe("RunInOwnGroup", func() {
	It("starts the command in its own process group and interrupts it with a grace period", func() {
		cmd := exec.CommandContext(context.Background(), "/nonexistent/k0sctl", "apply")

		err := system.RunInOwnGroup(cmd)

		Expect(err).To(MatchError(ContainSubstring("failed to start /nonexistent/k0sctl")))
		Expect(cmd.SysProcAttr.Setpgid).To(BeTrue())
		Expect(cmd.Cancel).NotTo(BeNil())
		Expect(cmd.WaitDelay).To(Equal(system.InterruptGracePeriod))
	})
})
