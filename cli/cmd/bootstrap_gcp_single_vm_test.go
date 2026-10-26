// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

var _ = Describe("Single VM bootstrap command", func() {
	It("registers single VM options on the existing GCP command", func() {
		root := GetRootCmd()
		command, _, err := root.Find([]string{"beta", "bootstrap-gcp"})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Flags().Lookup("single-vm").DefValue).To(Equal("false"))
		Expect(command.Flags().Lookup("install-local")).NotTo(BeNil())
		Expect(command.Flags().Lookup("install-version")).NotTo(BeNil())
		Expect(command.Flags().Lookup("machine-type").DefValue).To(BeEmpty())
		Expect(command.Flags().Lookup("spot-vms").DefValue).To(Equal("false"))
		Expect(command.Flags().Lookup("storage-engine").DefValue).To(Equal("rook-ceph"))
		Expect(command.Flags().Lookup("install-config-template")).NotTo(BeNil())
		Expect(command.Flags().Lookup("config")).NotTo(BeNil())
		Expect(command.Flags().Lookup("vault")).NotTo(BeNil())
		Expect(command.Flags().Lookup("priv-key")).NotTo(BeNil())
		Expect(command.Flags().Lookup("root-disk-size").DefValue).To(Equal("50"))

		beta, _, err := root.Find([]string{"beta"})
		Expect(err).NotTo(HaveOccurred())
		Expect(beta.Commands()).NotTo(ContainElement(HaveField("Use", "bootstrap-gcp-single-vm")))
	})

	It("validates single-VM requirements through the shared command before creating clients", func() {
		command, _, err := GetRootCmd().Find([]string{"beta", "bootstrap-gcp"})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Flags().Set("single-vm", "true")).To(Succeed())
		Expect(command.RunE(command, nil)).To(MatchError(ContainSubstring("--registry-user is required with --single-vm")))
	})

	It("keeps the Ceph disk filter internal", func() {
		command, _, err := GetRootCmd().Find([]string{"beta", "bootstrap-gcp"})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Flags().Lookup("ceph-device-filter")).To(BeNil())
	})

	It("keeps cleanup and restart under the shared command", func() {
		cleanup, _, err := GetRootCmd().Find([]string{"beta", "bootstrap-gcp", "cleanup"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cleanup.Name()).To(Equal("cleanup"))

		restart, _, err := GetRootCmd().Find([]string{"beta", "bootstrap-gcp", "restart-vms"})
		Expect(err).NotTo(HaveOccurred())
		Expect(restart.Name()).To(Equal("restart-vms"))
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

var _ = Describe("GCP bootstrap mode options", func() {
	var command *BootstrapGcpCmd

	BeforeEach(func() {
		GinkgoT().Setenv("OMS_REGISTRY_PASSWORD", "password")

		command = &BootstrapGcpCmd{
			cmd: &cobra.Command{},
			CodesphereEnv: &gcp.CodesphereEnvironment{
				RemoteOmsBinaryPath: "/oms", RegistryUser: "registry",
			},
		}
		command.addBootstrapModeFlags()
		command.cmd.Flags().Int64Var(&command.CodesphereEnv.RootDiskSize, "root-disk-size", 50, "")
		command.cmd.Flags().BoolVar(&command.CodesphereEnv.MultiDC, "multi-dc", false, "")
		command.cmd.Flags().BoolVar(&command.CodesphereEnv.Airgapped, "airgapped", false, "")
		command.cmd.Flags().BoolVar(&command.CodesphereEnv.InstallCodesphereOnly, "install-codesphere-only", false, "")
		command.cmd.Flags().StringVar(&command.CodesphereEnv.InstallConfigPath, "install-config", "config.yaml", "")
	})

	It("preserves multi-VM defaults without single-VM credentials", func() {
		GinkgoT().Setenv("OMS_REGISTRY_PASSWORD", "")

		command.CodesphereEnv.RemoteOmsBinaryPath = ""
		command.CodesphereEnv.RegistryUser = ""
		Expect(command.prepareBootstrapOptions()).To(Succeed())
		Expect(command.CodesphereEnv.RootDiskSize).To(Equal(int64(50)))
	})

	It("uses the same root disk default in single-VM mode", func() {
		Expect(command.cmd.Flags().Set("single-vm", "true")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(Succeed())
		Expect(command.CodesphereEnv.SingleVM).To(BeTrue())
		Expect(command.CodesphereEnv.RootDiskSize).To(Equal(int64(50)))
	})

	It("preserves an explicit larger single-VM disk size", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("root-disk-size", "100")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(Succeed())
		Expect(command.CodesphereEnv.RootDiskSize).To(Equal(int64(100)))
	})

	It("allows single-VM bootstrap without a local OMS binary", func() {
		command.CodesphereEnv.SingleVM = true
		command.CodesphereEnv.RemoteOmsBinaryPath = ""
		Expect(command.prepareBootstrapOptions()).To(Succeed())
	})

	It("requires a registry user in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true
		command.CodesphereEnv.RegistryUser = ""
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("--registry-user is required")))
	})

	It("requires the registry password in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true

		GinkgoT().Setenv("OMS_REGISTRY_PASSWORD", "")
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("OMS_REGISTRY_PASSWORD must be set")))
	})

	It("rejects an unsupported single-VM storage engine", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("storage-engine", "unknown")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("unsupported storage engine")))
	})

	It("accepts local storage and merged configuration in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("storage-engine", "local")).To(Succeed())
		Expect(command.cmd.Flags().Set("config", "base.yaml")).To(Succeed())
		Expect(command.cmd.Flags().Set("config", "override.yaml")).To(Succeed())
		Expect(command.cmd.Flags().Set("vault", "secrets.yaml")).To(Succeed())
		Expect(command.cmd.Flags().Set("priv-key", "age-key")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(Succeed())
		Expect(command.CodesphereEnv.InstallConfigs).To(Equal([]string{"base.yaml", "override.yaml"}))
	})

	It("accepts inherited global flags in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true
		root := &cobra.Command{}
		root.PersistentFlags().Bool("verbose", false, "")
		root.AddCommand(command.cmd)
		Expect(root.PersistentFlags().Set("verbose", "true")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(Succeed())
	})

	It("accepts config and vault inputs in multi-VM mode", func() {
		Expect(command.cmd.Flags().Set("install-config-template", "template.yaml")).To(Succeed())
		Expect(command.cmd.Flags().Set("config", "base.yaml")).To(Succeed())
		Expect(command.cmd.Flags().Set("vault", "secrets.yaml")).To(Succeed())
		Expect(command.cmd.Flags().Set("priv-key", "age-key")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(Succeed())
	})

	It("rejects a private key without a vault in either mode", func() {
		Expect(command.cmd.Flags().Set("priv-key", "age-key")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError("--priv-key requires --vault"))
		command.CodesphereEnv.SingleVM = true
		Expect(command.prepareBootstrapOptions()).To(MatchError("--priv-key requires --vault"))
	})

	It("rejects single-VM storage flags without selecting the mode", func() {
		Expect(command.cmd.Flags().Set("storage-engine", "local")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError("--storage-engine requires --single-vm"))
	})

	It("rejects a multi-data-center layout in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("multi-dc", "true")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("--multi-dc is not supported with --single-vm")))
	})

	It("rejects airgapped installation in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("airgapped", "true")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("--airgapped is not supported with --single-vm")))
	})

	It("rejects Codesphere-only installation in single-VM mode", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("install-codesphere-only", "true")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("--install-codesphere-only is not supported with --single-vm")))
	})

	It("rejects multi-VM config paths rather than ignoring them", func() {
		command.CodesphereEnv.SingleVM = true
		Expect(command.cmd.Flags().Set("install-config", "custom.yaml")).To(Succeed())
		Expect(command.prepareBootstrapOptions()).To(MatchError(ContainSubstring("--install-config is not supported with --single-vm")))
	})
})
