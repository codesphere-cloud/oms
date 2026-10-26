// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Local bootstrap registry configuration", func() {
	var (
		b     *LocalBootstrapper
		vault *files.InstallVault
	)

	BeforeEach(func() {
		config := files.NewRootConfig()
		vault = &files.InstallVault{}
		manager := installer.NewMockInstallConfigManager(GinkgoT())
		manager.EXPECT().GetVault().Return(vault)
		b = &LocalBootstrapper{
			icg: manager,
			Env: &CodesphereEnvironment{
				InstallConfig: &config, RegistryUser: "user", RegistryPassword: "password",
			},
		}
	})

	It("preserves the registry port and repository prefix in the install config", func() {
		b.Env.RegistryURL = "registry.example.com:5000/team/"
		b.Env.InstallConfig.PcApps = files.ChartValues{
			"chartsRegistry": "old.example.com/codesphere-cloud/charts",
			"applications":   map[string]any{"custom-app": map[string]any{"enabled": true}},
		}
		Expect(b.EnsureGitHubAccessConfigured()).To(Succeed())
		Expect(b.Env.InstallConfig.Registry.Server).To(Equal("registry.example.com:5000/team"))
		Expect(b.Env.InstallConfig.PcApps["chartsRegistry"]).To(Equal("registry.example.com:5000/team/codesphere-cloud/charts"))
		Expect(b.Env.InstallConfig.PcApps["applications"]).To(Equal(map[string]any{"custom-app": map[string]any{"enabled": true}}))
		Expect(b.Env.chartsRegistryURL()).To(Equal("registry.example.com:5000/team/codesphere-cloud/charts"))
		Expect(vault.GetSecret(files.SecretRegistryUsername).Fields.Password).To(Equal("user"))
		Expect(vault.GetSecret(files.SecretRegistryPassword).Fields.Password).To(Equal("password"))
	})

	It("preserves a registry repository path", func() {
		b.Env.RegistryURL = "registry.example.com/custom"
		Expect(b.EnsureGitHubAccessConfigured()).To(Succeed())
		Expect(b.Env.InstallConfig.Registry.Server).To(Equal("registry.example.com/custom"))
	})

	It("keeps the GitHub registry default when no URL is provided", func() {
		b.Env.InstallConfig.PcApps = nil
		Expect(b.EnsureGitHubAccessConfigured()).To(Succeed())
		Expect(b.Env.InstallConfig.Registry.Server).To(Equal("ghcr.io"))
		Expect(b.Env.InstallConfig.PcApps["chartsRegistry"]).To(Equal("ghcr.io/codesphere-cloud/charts"))
		Expect(b.Env.chartsRegistryURL()).To(Equal("ghcr.io/codesphere-cloud/charts"))
	})
})

var _ = Describe("Local bootstrap chart registry URL", func() {
	It("rejects an OCI scheme in the registry base", func() {
		env := &CodesphereEnvironment{RegistryURL: "oci://registry.example.com/team"}
		_, err := env.chartsRegistryURL()
		Expect(err).To(MatchError(ContainSubstring("must not include a scheme")))
	})

	It("rejects an HTTPS scheme in the registry base", func() {
		env := &CodesphereEnvironment{RegistryURL: "https://registry.example.com/team"}
		_, err := env.registryURL()
		Expect(err).To(MatchError(ContainSubstring("must not include a scheme")))
	})

	It("returns a parse error for an invalid registry port", func() {
		env := &CodesphereEnvironment{RegistryURL: "registry.example.com:invalid/mirror"}
		_, err := env.chartsRegistryURL()
		Expect(err).To(MatchError(ContainSubstring("parse chart registry URL")))
	})

	It("joins paths for a registry host with a port", func() {
		env := &CodesphereEnvironment{RegistryURL: "localhost:5000"}
		Expect(env.chartsRegistryURL()).To(Equal("localhost:5000/codesphere-cloud/charts"))
	})

	It("cleans repeated slashes while preserving nested repository prefixes", func() {
		env := &CodesphereEnvironment{RegistryURL: "registry.example.com/team//mirror/"}
		Expect(env.chartsRegistryURL()).To(Equal("registry.example.com/team/mirror/codesphere-cloud/charts"))
	})
})
