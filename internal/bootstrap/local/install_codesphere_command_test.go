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
	It("uses the shared vault secret name for both install phases", func() {
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{ArgoCDRegistryURL: "oci://ghcr.io/codesphere-cloud/charts"}, Verbose: true}
		opts := b.localInstallCodesphereOptions("/bundle", "/config.yaml", "/vault.yaml", "/age-key", "/config")

		Expect(opts.VaultSecretName).To(Equal(vault.VaultSecretName))
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
})
