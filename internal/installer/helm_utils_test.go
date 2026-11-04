// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Helm chart utilities", func() {
	var chart installer.ChartInfo

	BeforeEach(func() {
		chart = installer.ChartInfo{Name: "component", Version: "1.2.3", Dependencies: []string{"controller"}}
	})

	It("validates a pinned chart and its bundled dependency", func() {
		Expect(chart.Validate("component", "1.2.3", "controller")).To(Succeed())
	})
	It("rejects a different chart name", func() {
		Expect(chart.Validate("other", "1.2.3")).To(MatchError(ContainSubstring("does not match expected")))
	})
	It("rejects a version that differs from the pin", func() {
		Expect(chart.Validate("component", "1.2.4")).To(MatchError(ContainSubstring("does not match expected")))
	})
	It("rejects a missing bundled dependency", func() {
		Expect(chart.Validate("component", "1.2.3", "missing")).To(MatchError(ContainSubstring("missing its bundled missing dependency")))
	})
	It("rejects an invalid pinned version", func() {
		Expect(chart.Validate("component", "latest")).To(MatchError(ContainSubstring("invalid chart version")))
	})
	It("keeps HTTP repositories separate from the chart name", func() {
		name, repo := installer.ResolveChartReference("https://charts.example.com", "component")
		Expect(name).To(Equal("component"))
		Expect(repo).To(Equal("https://charts.example.com"))
	})
	It("expands an OCI chart reference and removes a trailing slash", func() {
		name, repo := installer.ResolveChartReference("oci://registry.example.com/charts/", "component")
		Expect(name).To(Equal("oci://registry.example.com/charts/component"))
		Expect(repo).To(BeEmpty())
	})
	It("preserves the registry port when extracting the host", func() {
		Expect(installer.RegistryHost("oci://registry.example.com:5000/team/component")).To(Equal("registry.example.com:5000"))
	})
	It("rejects unsupported repository schemes", func() {
		Expect(installer.ValidateChartRepositoryURL("file:///charts")).To(MatchError(ContainSubstring("must start with")))
	})
	It("compares a prerelease as older than its stable version", func() {
		comparison, err := installer.CompareChartVersions("1.2.3-rc.1", "1.2.3")
		Expect(err).NotTo(HaveOccurred())
		Expect(comparison).To(BeNumerically("<", 0))
	})
	It("authenticates an OCI chart using the extracted host and supplied credentials", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com:5000", "user", "token").Return(nil)
		Expect(installer.LoginChartRegistry(context.Background(), helm, "oci://registry.example.com:5000/team/component", "user", "token")).To(Succeed())
	})
	It("does not log in to an HTTP chart repository", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		Expect(installer.LoginChartRegistry(context.Background(), helm, "https://charts.example.com", "user", "token")).To(Succeed())
	})
})
