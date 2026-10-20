// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Single VM bootstrap command", func() {
	It("registers separately with its package, configuration and provisioning flags", func() {
		root := GetRootCmd()
		regular, _, err := root.Find([]string{"beta", "bootstrap-gcp"})
		Expect(err).NotTo(HaveOccurred())
		single, _, err := root.Find([]string{"beta", "bootstrap-gcp-single-vm"})
		Expect(err).NotTo(HaveOccurred())
		Expect(single).NotTo(BeIdenticalTo(regular))
		Expect(single.Name()).To(Equal("bootstrap-gcp-single-vm"))
		Expect(single.Flags().Lookup("install-local")).NotTo(BeNil())
		Expect(single.Flags().Lookup("install-version")).NotTo(BeNil())
		Expect(single.Flags().Lookup("machine-type")).NotTo(BeNil())
		Expect(single.Flags().Lookup("spot-vms").DefValue).To(Equal("false"))
		Expect(single.Flags().Lookup("storage-engine").DefValue).To(Equal("rook-ceph"))
		Expect(single.Flags().Lookup("install-config-template")).NotTo(BeNil())
		Expect(single.Flags().Lookup("config")).NotTo(BeNil())
		Expect(single.Flags().Lookup("vault")).NotTo(BeNil())
		Expect(single.Flags().Lookup("priv-key")).NotTo(BeNil())
	})

	It("supplies VM-specific settings through the local install config", func() {
		local, _, err := GetRootCmd().Find([]string{"beta", "bootstrap-local"})
		Expect(err).NotTo(HaveOccurred())
		Expect(local.Flags().Lookup("workspace-hosting-base-domain")).To(BeNil())
		Expect(local.Flags().Lookup("public-ip")).To(BeNil())
		Expect(local.Flags().Lookup("gateway-ip")).To(BeNil())
		Expect(local.Flags().Lookup("public-gateway-ip")).To(BeNil())
		Expect(local.Flags().Lookup("ssh-proxy-ip")).To(BeNil())
	})

	It("accepts Rook Ceph storage", func() {
		Expect(validateLocalStorageEngine("rook-ceph")).To(Succeed())
	})

	It("accepts local storage", func() {
		Expect(validateLocalStorageEngine("local")).To(Succeed())
	})

	It("rejects unsupported storage", func() {
		Expect(validateLocalStorageEngine("unknown")).To(MatchError(ContainSubstring("unsupported storage engine")))
	})
})

var _ = Describe("Single VM restart environment", func() {
	var (
		fw      *intutil.MockFileIO
		command *BootstrapGcpRestartVMsCmd
	)

	BeforeEach(func() {
		fw = intutil.NewMockFileIO(GinkgoT())
		command = &BootstrapGcpRestartVMsCmd{Opts: &BootstrapGcpRestartVMsOpts{}}
		path := gcp.GetInfraFilePath()
		fw.EXPECT().Exists(path).Return(true)
		fw.EXPECT().ReadFile(path).Return([]byte(`{"project_id":"project","zone":"zone","single_vm":true}`), nil)
	})

	It("restores the single VM layout from the infra file", func() {
		env, err := command.resolveEnvironment(fw)
		Expect(err).NotTo(HaveOccurred())
		Expect(env.SingleVM).To(BeTrue())
		Expect(env.ProjectID).To(Equal("project"))
		Expect(env.Zone).To(Equal("zone"))
		Expect(gcp.VMDefsForEnv(env)).To(ConsistOf(HaveField("Name", "codesphere")))
	})

	It("keeps the single VM layout when project and zone are explicit", func() {
		command.Opts.ProjectID = "other-project"
		command.Opts.Zone = "other-zone"
		env, err := command.resolveEnvironment(fw)
		Expect(err).NotTo(HaveOccurred())
		Expect(env.SingleVM).To(BeTrue())
		Expect(env.ProjectID).To(Equal("other-project"))
		Expect(env.Zone).To(Equal("other-zone"))
		Expect(gcp.VMDefsForEnv(env)).To(ConsistOf(HaveField("Name", "codesphere")))
	})
})
