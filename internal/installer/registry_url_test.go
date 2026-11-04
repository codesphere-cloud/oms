// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DefaultChartsRegistry", func() {
	It("writes the configured mirror and repository prefix into pc-apps values", func() {
		config := &files.RootConfig{Registry: &files.RegistryConfig{Server: "mirror.example.com:5000/team"}}
		Expect(installer.DefaultChartsRegistry(config)).To(Succeed())
		Expect(config.PcApps).To(HaveKeyWithValue("chartsRegistry", "mirror.example.com:5000/team/codesphere-cloud/charts"))
	})

	It("preserves an explicit chart repository", func() {
		config := &files.RootConfig{
			Registry: &files.RegistryConfig{Server: "mirror.example.com"},
			PcApps:   files.ChartValues{"chartsRegistry": "custom.example.com/charts"},
		}
		Expect(installer.DefaultChartsRegistry(config)).To(Succeed())
		Expect(config.PcApps).To(HaveKeyWithValue("chartsRegistry", "custom.example.com/charts"))
	})

	It("leaves the chart default in place when no mirror is configured", func() {
		config := &files.RootConfig{}
		Expect(installer.DefaultChartsRegistry(config)).To(Succeed())
		Expect(config.PcApps).To(BeEmpty())
	})

	It("rejects an invalid mirror address", func() {
		config := &files.RootConfig{Registry: &files.RegistryConfig{Server: "https://mirror.example.com"}}
		Expect(installer.DefaultChartsRegistry(config)).To(MatchError(ContainSubstring("must not include a scheme")))
	})
})

var _ = Describe("ChartRegistryURL", func() {
	It("keeps the BOM chart repository when no registry server is configured", func() {
		url, err := installer.ChartRegistryURL("")
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeEmpty())
	})

	It("joins a registry host with a port", func() {
		url, err := installer.ChartRegistryURL("registry.example.com:5000")
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(Equal("registry.example.com:5000/codesphere-cloud/charts"))
	})

	It("accepts an IP address used by local registries", func() {
		url, err := installer.ChartRegistryURL("10.10.0.2:5000/")
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(Equal("10.10.0.2:5000/codesphere-cloud/charts"))
	})

	It("cleans a trailing slash without losing the repository prefix", func() {
		url, err := installer.ChartRegistryURL("registry.example.com/team/")
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(Equal("registry.example.com/team/codesphere-cloud/charts"))
	})

	It("cleans repeated path separators", func() {
		url, err := installer.ChartRegistryURL("registry.example.com/team//mirror/")
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(Equal("registry.example.com/team/mirror/codesphere-cloud/charts"))
	})

	It("rejects a registry server containing a scheme", func() {
		_, err := installer.ChartRegistryURL("https://registry.example.com")
		Expect(err).To(MatchError(ContainSubstring("must not include a scheme")))
	})

	It("rejects an invalid port", func() {
		_, err := installer.ChartRegistryURL("registry.example.com:port")
		Expect(err).To(MatchError(ContainSubstring("parse chart registry URL")))
	})
})
