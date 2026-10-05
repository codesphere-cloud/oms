// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package steps_test

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/codesphere/testplan/steps"
)

var _ = Describe("Catalog", func() {
	It("contains every step file in this folder", func() {
		Expect(steps.Names()).To(ConsistOf(steps.StatusName, steps.SmoketestName))
	})

	It("describes every step, so listings and progress logs are never blank", func() {
		for _, s := range steps.Catalog() {
			Expect(s.Name).NotTo(BeEmpty())
			Expect(s.Description).NotTo(BeEmpty(), "step %q has no description", s.Name)
			Expect(s.Run).NotTo(BeNil(), "step %q has no run function", s.Name)
		}
	})

	It("lists the steps in a stable order, independent of file initialization order", func() {
		Expect(steps.Names()).To(Equal(slices.Sorted(slices.Values(steps.Names()))))
	})
})

var _ = Describe("Playlists", func() {
	It("offers a default and a readiness playlist", func() {
		Expect(steps.PlaylistNames()).To(ContainElements(steps.DefaultPlaylist, steps.ReadinessPlaylist))
	})

	It("only references steps that exist in this folder", func() {
		registry := steps.Registry(&steps.Options{})

		for _, p := range steps.Playlists() {
			selected, err := registry.SelectPlaylist(p.Name)

			Expect(err).NotTo(HaveOccurred(), "playlist %q references an unknown step", p.Name)
			Expect(selected).To(HaveLen(len(p.Tests)))
		}
	})

	It("keeps the order the playlist defines", func() {
		selected, err := steps.Registry(&steps.Options{}).SelectPlaylist(steps.DefaultPlaylist)

		Expect(err).NotTo(HaveOccurred())
		Expect(selected[0].Name()).To(Equal(steps.StatusName))
		Expect(selected[1].Name()).To(Equal(steps.SmoketestName))
	})
})
