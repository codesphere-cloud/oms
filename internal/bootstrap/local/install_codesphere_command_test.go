// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Local Codesphere installer options", func() {
	It("configures local components and forwards bootstrap settings", func() {
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{RegistryURL: "ghcr.io"}, Verbose: true}
		opts, err := b.localInstallCodesphereOptions("/bundle", "/config.yaml", "/vault.yaml", "/age-key", "/config")
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.LocalComponents).To(BeTrue())
		Expect(opts.AutoApprove).To(BeTrue())
		Expect(opts.Verbose).To(BeTrue())
		Expect(opts.Package).To(Equal("/bundle"))
		Expect(opts.Configs).To(Equal([]string{"/config.yaml"}))
		Expect(opts.Vault).To(Equal("/vault.yaml"))
		Expect(opts.PrivKey).To(Equal("/age-key"))
		Expect(opts.VaultType).To(Equal(string(vault.TypeSOPS)))
		Expect(opts.LocalConfigDir).To(Equal("/config"))
		Expect(opts.ArgoCDRegistryURL).To(Equal("ghcr.io/codesphere-cloud/charts"))
		Expect(opts.ArgoCDVersion).To(Equal("9.5.21"))
		Expect(opts.ArgoCDForceConflicts).To(BeTrue())
		Expect(opts.ArgoCDRepoURL).To(Equal(argocd.DefaultRepoURL))
	})

	It("uses the vault secret name required by the dependencies phase", func() {
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{}}
		opts, err := b.localInstallCodesphereOptions("/bundle", "/config.yaml", "/vault.yaml", "/age-key", "/config")
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.VaultSecretName).To(Equal(vault.VaultSecretName))
	})

	It("appends the chart path to a registry with a port and repository prefix", func() {
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{RegistryURL: "registry.example.com:5000/mirror/"}}
		opts, err := b.localInstallCodesphereOptions("/bundle", "/config.yaml", "/vault.yaml", "/age-key", "/config")
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.ArgoCDRegistryURL).To(Equal("registry.example.com:5000/mirror/codesphere-cloud/charts"))
	})
})
