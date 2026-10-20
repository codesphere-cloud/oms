// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("K0sCleanup", func() {
	Describe("K0sCleanupNodeIPs", func() {
		It("returns control planes and workers without duplicates", func() {
			installConfig := newTestConfig("test-dc", true, "10.0.1.10", "10.0.1.11")
			installConfig.Kubernetes.Workers = []files.K8sNode{
				{IPAddress: "10.0.1.11"},
				{IPAddress: "10.0.1.20"},
			}

			Expect(installer.K0sCleanupNodeIPs(installConfig)).To(Equal([]string{"10.0.1.10", "10.0.1.11", "10.0.1.20"}))
		})

		It("returns workers when there are no control planes", func() {
			installConfig := newTestConfig("test-dc", true)
			installConfig.Kubernetes.Workers = []files.K8sNode{{IPAddress: "10.0.1.20"}}

			Expect(installer.K0sCleanupNodeIPs(installConfig)).To(Equal([]string{"10.0.1.20"}))
		})

		It("returns nothing for an install-config without nodes", func() {
			Expect(installer.K0sCleanupNodeIPs(newTestConfig("test-dc", true))).To(BeEmpty())
		})
	})

	Describe("K0sCleanupScript", func() {
		It("contains every cleanup step", func() {
			script := installer.K0sCleanupScript()

			Expect(script).To(ContainSubstring("for svc in k0sworker k0scontroller"))
			Expect(script).To(ContainSubstring(`systemctl stop "$svc.service"`))
			Expect(script).To(ContainSubstring(`systemctl disable "$svc.service"`))
			Expect(script).To(ContainSubstring("K0S=" + installer.K0sPackageBinaryPath))
			Expect(script).To(ContainSubstring(`"$K0S" reset`))
			Expect(script).To(ContainSubstring("rm -f /etc/systemd/system/k0sworker.service /etc/systemd/system/k0scontroller.service || exit 1"))
			Expect(script).To(ContainSubstring("for dir in /var/lib/k0s /etc/k0s; do"))
			Expect(script).To(ContainSubstring(`rm -rf --one-file-system "$dir" || exit 1`))
			Expect(script).To(ContainSubstring("systemctl daemon-reload"))
		})

		It("unmounts only volumes below the k0s directories and refuses to delete through them", func() {
			script := installer.K0sCleanupScript()

			Expect(script).To(ContainSubstring(`'$2 ~ "^/(var/lib/k0s|etc/k0s)/"`))

			unmount := strings.Index(script, `umount "$mnt"`)
			mountCheck := strings.Index(script, `if [ -n "$(k0s_nested_mounts)" ]; then`)
			deletion := strings.Index(script, "rm -rf")

			Expect(unmount).To(BeNumerically(">", 0))
			Expect(mountCheck).To(BeNumerically(">", unmount))
			Expect(deletion).To(BeNumerically(">", mountCheck))
			Expect(script[mountCheck:deletion]).To(ContainSubstring("exit 1"))
		})

		It("keeps dedicated k0s mounts and deletes only their contents", func() {
			script := installer.K0sCleanupScript()

			Expect(script).To(ContainSubstring(`if mountpoint -q "$dir"; then`))
			Expect(script).To(ContainSubstring(`find "$dir" -mindepth 1 -maxdepth 1 -exec rm -rf --one-file-system {} + || exit 1`))
		})
	})
})
