// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system_test

import (
	"errors"
	"os"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/system"
)

var _ = Describe("ProcessGroups", func() {
	var (
		signaller *system.MockGroupSignaller
		groups    *system.ProcessGroups
	)

	BeforeEach(func() {
		signaller = system.NewMockGroupSignaller(GinkgoT())
		groups = system.NewProcessGroups(signaller)
	})

	It("interrupts a group only once", func() {
		signaller.EXPECT().SignalGroup(42, os.Interrupt).Return(nil).Once()
		groups.Add(42)

		Expect(groups.Interrupt(42)).To(Succeed())
		Expect(groups.Interrupt(42)).To(Succeed())
	})

	It("reports an exited group as done", func() {
		signaller.EXPECT().SignalGroup(42, os.Interrupt).Return(syscall.ESRCH).Once()
		groups.Add(42)

		Expect(groups.Interrupt(42)).To(MatchError(os.ErrProcessDone))
	})

	It("forwards only the forcing signal to an interrupted group", func() {
		signaller.EXPECT().SignalGroup(42, os.Interrupt).Return(nil).Once()
		signaller.EXPECT().SignalGroup(42, syscall.SIGTERM).Return(nil).Once()
		groups.Add(42)
		Expect(groups.Interrupt(42)).To(Succeed())

		groups.Force(syscall.SIGTERM)
	})

	It("interrupts a group before forcing it when the cancellation has not reached it yet", func() {
		var sent []os.Signal

		signaller.EXPECT().SignalGroup(42, os.Interrupt).RunAndReturn(func(_ int, sig os.Signal) error {
			sent = append(sent, sig)
			return nil
		}).Times(2)
		groups.Add(42)

		groups.Force(os.Interrupt)
		Expect(groups.Interrupt(42)).To(Succeed())

		Expect(sent).To(Equal([]os.Signal{os.Interrupt, os.Interrupt}))
	})

	It("keeps a group interrupted before it was added", func() {
		signaller.EXPECT().SignalGroup(42, os.Interrupt).Return(nil).Once()

		Expect(groups.Interrupt(42)).To(Succeed())
		groups.Add(42)
		Expect(groups.Interrupt(42)).To(Succeed())
	})

	It("does not force a group that could not be interrupted", func() {
		signaller.EXPECT().SignalGroup(42, os.Interrupt).Return(errors.New("permission denied")).Once()
		groups.Add(42)

		groups.Force(syscall.SIGTERM)
	})

	It("does not signal removed groups", func() {
		groups.Add(42)
		groups.Remove(42)

		groups.Force(os.Interrupt)
	})
})
