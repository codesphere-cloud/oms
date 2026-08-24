// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ceph device selection", func() {
	It("uses all devices by default", func() {
		selection := (&LocalBootstrapper{Env: &CodesphereEnvironment{}}).cephDeviceSelection()
		Expect(selection.UseAllDevices).NotTo(BeNil())
		Expect(*selection.UseAllDevices).To(BeTrue())
		Expect(selection.DeviceFilter).To(BeEmpty())
		Expect(selection.DevicePathFilter).To(BeEmpty())
	})

	It("uses only the configured device-name filter", func() {
		selection := (&LocalBootstrapper{Env: &CodesphereEnvironment{CephDeviceFilter: "^sd[b-z]$"}}).cephDeviceSelection()
		Expect(*selection.UseAllDevices).To(BeFalse())
		Expect(selection.DeviceFilter).To(Equal("^sd[b-z]$"))
		Expect(selection.DevicePathFilter).To(BeEmpty())
	})

	It("uses only the configured device-path filter", func() {
		selection := (&LocalBootstrapper{Env: &CodesphereEnvironment{CephDevicePathFilter: "^/dev/disk/by-id/"}}).cephDeviceSelection()
		Expect(*selection.UseAllDevices).To(BeFalse())
		Expect(selection.DeviceFilter).To(BeEmpty())
		Expect(selection.DevicePathFilter).To(Equal("^/dev/disk/by-id/"))
	})

	It("passes both explicit filters to Rook", func() {
		selection := (&LocalBootstrapper{Env: &CodesphereEnvironment{
			CephDeviceFilter:     "^nvme",
			CephDevicePathFilter: "^/dev/disk/by-id/",
		}}).cephDeviceSelection()
		Expect(*selection.UseAllDevices).To(BeFalse())
		Expect(selection.DeviceFilter).To(Equal("^nvme"))
		Expect(selection.DevicePathFilter).To(Equal("^/dev/disk/by-id/"))
	})
})
