// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package datacenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
)

// The methods of DataCenter are asserted against the real layout in the gcp package's
// BuildDataCenters test, so only the path derivation is covered here.
var _ = Describe("SuffixedPath", func() {
	DescribeTable("inserts the suffix before the extension",
		func(path, suffix, expected string) {
			Expect(datacenter.SuffixedPath(path, suffix)).To(Equal(expected))
		},
		Entry("primary keeps its path", "config.yaml", "", "config.yaml"),
		Entry("single extension", "config.yaml", "-dc2", "config-dc2.yaml"),
		// prod.vault.yaml must become prod-dc2.vault.yaml, not prod.vault-dc2.yaml, so the
		// suffix goes before the first dot rather than the last.
		Entry("compound extension", "prod.vault.yaml", "-dc2", "prod-dc2.vault.yaml"),
		Entry("no extension", "config", "-dc2", "config-dc2"),
		Entry("absolute path", "/etc/codesphere/config.yaml", "-dc2", "/etc/codesphere/config-dc2.yaml"),
	)
})

var _ = Describe("RemoteK0sctlConfigPath", func() {
	DescribeTable("places the k0sctl config next to the install config",
		func(remoteConfigPath, suffix, expected string) {
			dc := &datacenter.DataCenter{RemoteConfigPath: remoteConfigPath, Suffix: suffix}
			Expect(dc.RemoteK0sctlConfigPath()).To(Equal(expected))
		},
		Entry("primary", "/etc/codesphere/config.yaml", "", "/etc/codesphere/k0sctl-config.yaml"),
		Entry("secondary", "/etc/codesphere/config-dc2.yaml", "-dc2", "/etc/codesphere/k0sctl-config-dc2.yaml"),
	)
})
