// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"os"

	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/dns/v1"
)

func protoString(s string) *string { return &s }

var _ = Describe("NewGCPBootstrapper", func() {
	It("creates a valid GCPBootstrapper", func() {
		env := env.NewEnv()
		Expect(env).NotTo(BeNil())

		ctx := context.Background()
		csEnv := &gcp.CodesphereEnvironment{}
		stlog := bootstrap.NewStepLogger(false)

		icg := installer.NewMockInstallConfigManager(GinkgoT())
		gc := gcp.NewMockGCPClientManager(GinkgoT())
		fw := util.NewMockFileIO(GinkgoT())
		nm := node.NewMockNodeManager(GinkgoT())

		bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
		Expect(err).NotTo(HaveOccurred())
		Expect(bs).NotTo(BeNil())
	})
})

var _ = Describe("Bootstrap", func() {
	var (
		e     env.Env
		ctx   context.Context
		csEnv *gcp.CodesphereEnvironment
		icg   *installer.MockInstallConfigManager
		gc    *gcp.MockGCPClientManager
		fw    *util.MockFileIO
		nm    *node.MockNodeManager
		bs    *gcp.GCPBootstrapper
	)

	BeforeEach(func() {
		e = env.NewEnv()
		ctx = context.Background()
		csEnv = &gcp.CodesphereEnvironment{
			InstallConfigPath: "fake-config-file",
			SecretsFilePath:   "fake-secret",
			ProjectName:       "test-project",
			BillingAccount:    "test-billing-account",
			Region:            "us-central1",
			Zone:              "us-central1-a",
			BaseDomain:        "example.com",
			DNSProjectID:      "dns-project",
			DNSZoneName:       "test-zone",
		}
		stlog := bootstrap.NewStepLogger(false)

		icg = installer.NewMockInstallConfigManager(GinkgoT())
		gc = gcp.NewMockGCPClientManager(GinkgoT())
		fw = util.NewMockFileIO(GinkgoT())
		nm = node.NewMockNodeManager(GinkgoT())

		var err error
		bs, err = gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, nm, fw)
		Expect(err).NotTo(HaveOccurred())
	})

	It("runs bootstrap successfully", func() {
		bs.Env.RegistryType = gcp.RegistryTypeArtifactRegistry
		bs.Env.WriteConfig = true
		bs.Env.SecretsDir = "/secrets"

		// 1. EnsureInstallConfig
		fw.EXPECT().Exists("fake-config-file").Return(false)
		icg.EXPECT().ApplyProfile("dev").Return(nil)
		// Returning a real install config to avoid nil pointer dereferences later
		icg.EXPECT().GetInstallConfig().RunAndReturn(func() *files.RootConfig {
			realIcm := installer.NewInstallConfigManager()
			realIcm.ApplyProfile("dev")
			return realIcm.GetInstallConfig()
		})

		// 2. EnsureSecrets
		fw.EXPECT().Exists("fake-secret").Return(false)
		icg.EXPECT().GetVault().Return(&files.InstallVault{})

		// 3. EnsureProject
		gc.EXPECT().GetProjectByName(mock.Anything, "test-project").Return(nil, fmt.Errorf("project not found: test-project"))
		gc.EXPECT().CreateProjectID("test-project").Return("test-project-id")
		gc.EXPECT().CreateProject(mock.Anything, mock.Anything, "test-project").Return(mock.Anything, nil)

		// 4. EnsureBilling
		gc.EXPECT().GetBillingInfo("test-project-id").Return(&cloudbilling.ProjectBillingInfo{BillingEnabled: false}, nil)
		gc.EXPECT().EnableBilling("test-project-id", "test-billing-account").Return(nil)

		// 5. EnsureAPIsEnabled
		gc.EXPECT().EnableAPIs("test-project-id", mock.Anything).Return(nil)

		// 6. EnsureArtifactRegistry
		gc.EXPECT().GetArtifactRegistry("test-project-id", "us-central1", "codesphere-registry").Return(nil, fmt.Errorf("not found"))
		gc.EXPECT().CreateArtifactRegistry("test-project-id", "us-central1", "codesphere-registry").Return(&artifactregistrypb.Repository{Name: "codesphere-registry"}, nil)

		// 7. EnsureServiceAccounts
		gc.EXPECT().CreateServiceAccount("test-project-id", "cloud-controller", "cloud-controller").Return("cloud-controller@p.iam.gserviceaccount.com", false, nil)
		gc.EXPECT().CreateServiceAccount("test-project-id", "artifact-registry-writer", "artifact-registry-writer").Return("writer@p.iam.gserviceaccount.com", true, nil)
		gc.EXPECT().CreateServiceAccountKey("test-project-id", "writer@p.iam.gserviceaccount.com").Return("fake-key", nil)

		// 8. EnsureIAMRoles
		gc.EXPECT().AssignIAMRole("test-project-id", "artifact-registry-writer", "roles/artifactregistry.writer").Return(nil)
		gc.EXPECT().AssignIAMRole("test-project-id", "cloud-controller", "roles/compute.admin").Return(nil)

		// 9. EnsureVPC
		gc.EXPECT().CreateVPC("test-project-id", "us-central1", "test-project-id-vpc", "test-project-id-us-central1-subnet", "test-project-id-router", "test-project-id-nat-gateway").Return(nil)

		// 10. EnsureFirewallRules (5 times)
		gc.EXPECT().CreateFirewallRule("test-project-id", mock.Anything).Return(nil).Times(5)

		// 11. EnsureComputeInstances
		gc.EXPECT().CreateInstance("test-project-id", "us-central1-a", mock.Anything).Return(nil).Times(9)
		// GetInstance calls to retrieve IPs
		ipResp := &computepb.Instance{
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					NetworkIP: protoString("10.0.0.1"),
					AccessConfigs: []*computepb.AccessConfig{
						{NatIP: protoString("1.2.3.4")},
					},
				},
			},
		}
		gc.EXPECT().GetInstance("test-project-id", "us-central1-a", mock.Anything).Return(ipResp, nil).Times(9)
		fw.EXPECT().ReadFile(mock.Anything).Return([]byte("fake-key"), nil).Times(9)
		// UpdateNode is called once for the jumpbox to set its name and IPs
		nm.EXPECT().UpdateNode("jumpbox", "1.2.3.4", "10.0.0.1")
		// CreateSubNode is called 8 times for the other nodes
		nm.EXPECT().CreateSubNode("postgres", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("ceph-1", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("ceph-2", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("ceph-3", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("ceph-4", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("k0s-1", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("k0s-2", "1.2.3.4", "10.0.0.1").Return(nm)
		nm.EXPECT().CreateSubNode("k0s-3", "1.2.3.4", "10.0.0.1").Return(nm)

		nm.EXPECT().GetName().Return("mocknode").Maybe()
		nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
		nm.EXPECT().GetExternalIP().Return("1.2.3.4").Maybe()

		// 12. EnsureGatewayIPAddresses
		gc.EXPECT().GetAddress("test-project-id", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
		gc.EXPECT().CreateAddress("test-project-id", "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "gateway" })).Return("1.1.1.1", nil)
		gc.EXPECT().GetAddress("test-project-id", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
		gc.EXPECT().GetAddress("test-project-id", "us-central1", "public-gateway").Return(nil, fmt.Errorf("not found"))
		gc.EXPECT().CreateAddress("test-project-id", "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "public-gateway" })).Return("2.2.2.2", nil)
		gc.EXPECT().GetAddress("test-project-id", "us-central1", "public-gateway").Return(&computepb.Address{Address: protoString("2.2.2.2")}, nil)

		// 13. EnsureRootLoginEnabled
		nm.EXPECT().WaitForSSH(30 * time.Second).Return(nil).Times(9)
		nm.EXPECT().HasRootLoginEnabled().Return(false).Times(9)
		nm.EXPECT().EnableRootLogin().Return(nil).Times(9)

		// 14. EnsureJumpboxConfigured
		nm.EXPECT().HasAcceptEnvConfigured().Return(false)
		nm.EXPECT().ConfigureAcceptEnv().Return(nil)
		nm.EXPECT().HasCommand("oms-cli").Return(false)
		nm.EXPECT().InstallOms().Return(nil)

		// 15. EnsureInotifyWatches
		nm.EXPECT().HasInotifyWatchesConfigured().Return(false)
		nm.EXPECT().ConfigureInotifyWatches().Return(nil)
		nm.EXPECT().HasMemoryMapConfigured().Return(false)
		nm.EXPECT().ConfigureMemoryMap().Return(nil)

		// 16. UpdateInstallConfig
		icg.EXPECT().GenerateSecrets().Return(nil)
		icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
		icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
		nm.EXPECT().CopyFile("fake-config-file", "/etc/codesphere/config.yaml").Return(nil)
		nm.EXPECT().CopyFile("fake-secret", "/secrets/prod.vault.yaml").Return(nil)

		// 17. EnsureAgeKey
		nm.EXPECT().HasFile("/secrets/age_key.txt").Return(false)
		nm.EXPECT().RunSSHCommand("root", "mkdir -p /secrets; age-keygen -o /secrets/age_key.txt", true).Return(nil)

		// 18. EncryptVault
		nm.EXPECT().RunSSHCommand("root", "cp /secrets/prod.vault.yaml{,.bak}", true).Return(nil)
		nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
			return strings.Contains(cmd, "sops --encrypt")
		}), true).Return(nil)

		// 19. EnsureDNSRecords
		gc.EXPECT().EnsureDNSManagedZone("dns-project", "test-zone", "example.com.", mock.Anything).Return(nil)
		gc.EXPECT().EnsureDNSRecordSets("dns-project", "test-zone", mock.MatchedBy(func(records []*dns.ResourceRecordSet) bool {
			return len(records) == 4
		})).Return(nil)

		// 20. GenerateK0sConfigScript
		fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
		nm.EXPECT().CopyFile("configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
		nm.EXPECT().RunSSHCommand("root", "chmod +x /root/configure-k0s.sh", true).Return(nil)

		err := bs.Bootstrap()
		Expect(err).NotTo(HaveOccurred())
		Expect(bs.Env).NotTo(BeNil())
		Expect(bs.Env.ProjectID).To(HavePrefix("test-project-"))

		// Verify nodes are properly set in the environment
		Expect(bs.Env.Jumpbox).NotTo(BeNil(), "Jumpbox should be created")
		Expect(bs.Env.PostgreSQLNode).NotTo(BeNil(), "PostgreSQL node should be created")
		Expect(bs.Env.CephNodes).To(HaveLen(4), "Should have 4 Ceph nodes")
		Expect(bs.Env.ControlPlaneNodes).To(HaveLen(3), "Should have 3 K0s control plane nodes")

		// Verify mock returns expected values
		Expect(bs.Env.Jumpbox.GetName()).To(Equal("mocknode"))
		Expect(bs.Env.Jumpbox.GetExternalIP()).To(Equal("1.2.3.4"))
		Expect(bs.Env.Jumpbox.GetInternalIP()).To(Equal("10.0.0.1"))

		Expect(bs.Env.PostgreSQLNode.GetName()).To(Equal("mocknode"))
		Expect(bs.Env.PostgreSQLNode.GetExternalIP()).To(Equal("1.2.3.4"))
		Expect(bs.Env.PostgreSQLNode.GetInternalIP()).To(Equal("10.0.0.1"))

		for _, cephNode := range bs.Env.CephNodes {
			Expect(cephNode.GetName()).To(Equal("mocknode"))
			Expect(cephNode.GetExternalIP()).To(Equal("1.2.3.4"))
			Expect(cephNode.GetInternalIP()).To(Equal("10.0.0.1"))
		}

		for _, cpNode := range bs.Env.ControlPlaneNodes {
			Expect(cpNode.GetName()).To(Equal("mocknode"))
			Expect(cpNode.GetExternalIP()).To(Equal("1.2.3.4"))
			Expect(cpNode.GetInternalIP()).To(Equal("10.0.0.1"))
		}
	})
})

var _ = Describe("EnsureInstallConfig", func() {
	Describe("Valid EnsureInstallConfig", func() {
		It("uses existing when config file exists", func() {
			env := env.NewEnv()
			Expect(env).NotTo(BeNil())

			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallConfigPath: "existing-config-file",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("existing-config-file").Return(true)
			icg.EXPECT().LoadInstallConfigFromFile("existing-config-file").Return(nil)
			icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

			err = bs.EnsureInstallConfig()
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates install config when missing", func() {
			env := env.NewEnv()
			Expect(env).NotTo(BeNil())

			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallConfigPath: "nonexistent-config-file",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("nonexistent-config-file").Return(false)
			icg.EXPECT().ApplyProfile("dev").Return(nil)
			icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

			err = bs.EnsureInstallConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.InstallConfigPath).To(Equal("nonexistent-config-file"))
			Expect(bs.Env.InstallConfig).NotTo(BeNil())
		})
	})

	Describe("Invalid cases", func() {
		It("returns error when config file exists but fails to load", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallConfigPath: "existing-bad-config",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("existing-bad-config").Return(true)
			icg.EXPECT().LoadInstallConfigFromFile("existing-bad-config").Return(fmt.Errorf("bad format"))

			err = bs.EnsureInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load config file"))
			Expect(err.Error()).To(ContainSubstring("bad format"))
		})

		It("returns error when config file missing and applying profile fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallConfigPath: "missing-config",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("missing-config").Return(false)
			icg.EXPECT().ApplyProfile("dev").Return(fmt.Errorf("profile error"))

			err = bs.EnsureInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to apply profile"))
			Expect(err.Error()).To(ContainSubstring("profile error"))
		})
	})
})

var _ = Describe("EnsureSecrets", func() {
	Describe("Valid EnsureSecrets", func() {
		It("loads existing secrets file", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsFilePath: "existing-secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("existing-secrets").Return(true)
			icg.EXPECT().LoadVaultFromFile("existing-secrets").Return(nil)
			icg.EXPECT().MergeVaultIntoConfig().Return(nil)
			icg.EXPECT().GetVault().Return(&files.InstallVault{})

			err = bs.EnsureSecrets()
			Expect(err).NotTo(HaveOccurred())
		})

		It("skips when secrets file missing", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsFilePath: "missing-secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("missing-secrets").Return(false)
			icg.EXPECT().GetVault().Return(&files.InstallVault{})

			err = bs.EnsureSecrets()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("returns error when secrets file load fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsFilePath: "bad-secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("bad-secrets").Return(true)
			icg.EXPECT().LoadVaultFromFile("bad-secrets").Return(fmt.Errorf("load error"))

			err = bs.EnsureSecrets()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load vault file"))
			Expect(err.Error()).To(ContainSubstring("load error"))
		})

		It("returns error when merge fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsFilePath: "merr-secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().Exists("merr-secrets").Return(true)
			icg.EXPECT().LoadVaultFromFile("merr-secrets").Return(nil)
			icg.EXPECT().MergeVaultIntoConfig().Return(fmt.Errorf("merge error"))

			err = bs.EnsureSecrets()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to merge vault into config"))
			Expect(err.Error()).To(ContainSubstring("merge error"))
		})
	})
})

var _ = Describe("EnsureProject", func() {
	Describe("Valid EnsureProject", func() {
		It("uses existing project", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectName: "existing-proj",
				FolderID:    "123",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetProjectByName("123", "existing-proj").Return(&resourcemanagerpb.Project{ProjectId: "existing-id", Name: "existing-proj"}, nil)

			err = bs.EnsureProject()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.ProjectID).To(Equal("existing-id"))
		})

		It("creates project when missing", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectName: "new-proj",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetProjectByName("", "new-proj").Return(nil, fmt.Errorf("project not found: new-proj"))
			gc.EXPECT().CreateProjectID("new-proj").Return("new-proj-id")
			gc.EXPECT().CreateProject("", "new-proj-id", "new-proj").Return("", nil)

			err = bs.EnsureProject()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.ProjectID).To(Equal("new-proj-id"))
		})
	})

	Describe("Invalid cases", func() {
		It("returns error when GetProjectByName fails unexpectedly", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectName: "error-proj",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetProjectByName("", "error-proj").Return(nil, fmt.Errorf("api error"))

			err = bs.EnsureProject()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get project"))
			Expect(err.Error()).To(ContainSubstring("api error"))
		})

		It("returns error when CreateProject fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectName: "fail-create-proj",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetProjectByName("", "fail-create-proj").Return(nil, fmt.Errorf("project not found: fail-create-proj"))
			gc.EXPECT().CreateProjectID("fail-create-proj").Return("fail-create-proj-id")
			gc.EXPECT().CreateProject("", "fail-create-proj-id", "fail-create-proj").Return("", fmt.Errorf("create error"))

			err = bs.EnsureProject()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create project"))
			Expect(err.Error()).To(ContainSubstring("create error"))
		})
	})
})

var _ = Describe("EnsureBilling", func() {
	Describe("Valid EnsureBilling", func() {
		It("does nothing if billing already enabled correctly", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:      "pid",
				BillingAccount: "billing-123",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bi := &cloudbilling.ProjectBillingInfo{
				BillingEnabled:     true,
				BillingAccountName: "billing-123",
			}
			gc.EXPECT().GetBillingInfo("pid").Return(bi, nil)

			err = bs.EnsureBilling()
			Expect(err).NotTo(HaveOccurred())
		})

		It("enables billing if not enabled", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:      "pid",
				BillingAccount: "billing-123",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bi := &cloudbilling.ProjectBillingInfo{
				BillingEnabled: false,
			}
			gc.EXPECT().GetBillingInfo("pid").Return(bi, nil)
			gc.EXPECT().EnableBilling("pid", "billing-123").Return(nil)

			err = bs.EnsureBilling()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when GetBillingInfo fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetBillingInfo("pid").Return(nil, fmt.Errorf("billing info error"))

			err = bs.EnsureBilling()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get billing info"))
			Expect(err.Error()).To(ContainSubstring("billing info error"))
		})

		It("fails when EnableBilling fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:      "pid",
				BillingAccount: "acc",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bi := &cloudbilling.ProjectBillingInfo{
				BillingEnabled: false,
			}
			gc.EXPECT().GetBillingInfo("pid").Return(bi, nil)
			gc.EXPECT().EnableBilling("pid", "acc").Return(fmt.Errorf("enable error"))

			err = bs.EnsureBilling()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to enable billing"))
			Expect(err.Error()).To(ContainSubstring("enable error"))
		})
	})
})

var _ = Describe("EnsureAPIsEnabled", func() {
	Describe("Valid EnsureAPIsEnabled", func() {
		It("enables default APIs", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().EnableAPIs("pid", []string{
				"compute.googleapis.com",
				"serviceusage.googleapis.com",
				"artifactregistry.googleapis.com",
				"dns.googleapis.com",
			}).Return(nil)

			err = bs.EnsureAPIsEnabled()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when EnableAPIs fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().EnableAPIs("pid", mock.Anything).Return(fmt.Errorf("api error"))

			err = bs.EnsureAPIsEnabled()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to enable APIs"))
			Expect(err.Error()).To(ContainSubstring("api error"))
		})
	})
})

var _ = Describe("EnsureArtifactRegistry", func() {
	Describe("Valid EnsureArtifactRegistry", func() {
		It("uses existing registry if present", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			repo := &artifactregistrypb.Repository{Name: "projects/pid/locations/us-central1/repositories/codesphere-registry"}
			gc.EXPECT().GetArtifactRegistry("pid", "us-central1", "codesphere-registry").Return(repo, nil)

			err = bs.EnsureArtifactRegistry()
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates registry if missing", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetArtifactRegistry("pid", "us-central1", "codesphere-registry").Return(nil, fmt.Errorf("not found"))

			createdRepo := &artifactregistrypb.Repository{Name: "projects/pid/locations/us-central1/repositories/codesphere-registry"}
			gc.EXPECT().CreateArtifactRegistry("pid", "us-central1", "codesphere-registry").Return(createdRepo, nil)

			err = bs.EnsureArtifactRegistry()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when CreateArtifactRegistry fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetArtifactRegistry("pid", "us-central1", "codesphere-registry").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateArtifactRegistry("pid", "us-central1", "codesphere-registry").Return(nil, fmt.Errorf("create error"))

			err = bs.EnsureArtifactRegistry()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create artifact registry"))
			Expect(err.Error()).To(ContainSubstring("create error"))
		})
	})
})

var _ = Describe("EnsureLocalContainerRegistry", func() {
	Describe("Valid EnsureLocalContainerRegistry", func() {
		It("installs local registry", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Setup mocked node
			bs.Env.PostgreSQLNode = nm
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()

			// Check if running - return error to simulate not running
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "podman ps")
			}), true).Return(fmt.Errorf("not running"))

			// Install commands (8 commands) + scp/update-ca/docker commands (3 per 4 nodes = 12)
			nm.EXPECT().RunSSHCommand("root", mock.Anything, true).Return(nil).Times(8 + 12)

			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm}
			bs.Env.CephNodes = []node.NodeManager{nm, nm}

			nm.EXPECT().GetName().Return("mocknode").Maybe()

			err = bs.EnsureLocalContainerRegistry()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.InstallConfig.Registry.Username).To(Equal("custom-registry"))
		})
	})

	Describe("Invalid cases", func() {
		var (
			e     env.Env
			ctx   context.Context
			csEnv *gcp.CodesphereEnvironment
			icg   *installer.MockInstallConfigManager
			gc    *gcp.MockGCPClientManager
			fw    *util.MockFileIO
			nm    *node.MockNodeManager
			bs    *gcp.GCPBootstrapper
		)

		BeforeEach(func() {
			e = env.NewEnv()
			ctx = context.Background()
			csEnv = &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg = installer.NewMockInstallConfigManager(GinkgoT())
			gc = gcp.NewMockGCPClientManager(GinkgoT())
			fw = util.NewMockFileIO(GinkgoT())
			nm = node.NewMockNodeManager(GinkgoT())

			var err error
			bs, err = gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm}
			bs.Env.CephNodes = []node.NodeManager{nm, nm}

			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
		})

		It("fails when the 8th install command fails", func() {
			// First check - registry not running
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "podman ps")
			}), true).Return(fmt.Errorf("not running"))

			// First 7 install commands succeed
			nm.EXPECT().RunSSHCommand("root", mock.Anything, true).Return(nil).Times(7)

			// 8th install command fails
			nm.EXPECT().RunSSHCommand("root", mock.Anything, true).Return(fmt.Errorf("ssh error")).Once()

			err := bs.EnsureLocalContainerRegistry()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ssh error"))
		})

		It("fails when the first scp command fails", func() {
			// GetName is called in Logf
			nm.EXPECT().GetName().Return("mocknode").Maybe()

			// First check - registry not running
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "podman ps")
			}), true).Return(fmt.Errorf("not running"))

			// All 8 install commands succeed
			nm.EXPECT().RunSSHCommand("root", mock.Anything, true).Return(nil).Times(8)

			// First scp command fails
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.HasPrefix(cmd, "scp ")
			}), true).Return(fmt.Errorf("scp error")).Once()

			err := bs.EnsureLocalContainerRegistry()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy registry certificate"))
		})

		It("fails when update-ca-certificates fails", func() {
			// Override node setup for this test
			node1 := node.NewMockNodeManager(GinkgoT())
			bs.Env.ControlPlaneNodes = []node.NodeManager{node1}
			bs.Env.CephNodes = []node.NodeManager{}

			node1.EXPECT().GetInternalIP().Return("10.0.0.2").Maybe()
			node1.EXPECT().GetName().Return("node1").Maybe()

			// First check - registry not running
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "podman ps")
			}), true).Return(fmt.Errorf("not running"))

			// All 8 install commands succeed
			nm.EXPECT().RunSSHCommand("root", mock.Anything, true).Return(nil).Times(8)

			// scp succeeds
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.HasPrefix(cmd, "scp ")
			}), true).Return(nil).Once()

			// update-ca-certificates fails
			node1.EXPECT().RunSSHCommand("root", "update-ca-certificates", true).Return(fmt.Errorf("ca update error")).Once()

			err := bs.EnsureLocalContainerRegistry()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update CA certificates"))
		})

		It("fails when docker restart fails", func() {
			// Override node setup for this test
			node1 := node.NewMockNodeManager(GinkgoT())
			bs.Env.ControlPlaneNodes = []node.NodeManager{node1}
			bs.Env.CephNodes = []node.NodeManager{}

			node1.EXPECT().GetInternalIP().Return("10.0.0.2").Maybe()
			node1.EXPECT().GetName().Return("node1").Maybe()

			// First check - registry not running
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "podman ps")
			}), true).Return(fmt.Errorf("not running"))

			// All 8 install commands succeed
			nm.EXPECT().RunSSHCommand("root", mock.Anything, true).Return(nil).Times(8)

			// scp succeeds
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.HasPrefix(cmd, "scp ")
			}), true).Return(nil).Once()

			// update-ca-certificates succeeds
			node1.EXPECT().RunSSHCommand("root", "update-ca-certificates", true).Return(nil).Once()

			// docker restart fails
			node1.EXPECT().RunSSHCommand("root", "systemctl restart docker.service || true", true).Return(fmt.Errorf("docker restart error")).Once()

			err := bs.EnsureLocalContainerRegistry()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to restart docker service"))
		})
	})
})

var _ = Describe("EnsureServiceAccounts", func() {
	Describe("Valid EnsureServiceAccounts", func() {
		It("creates cloud-controller and skips writer if not artifact registry", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:    "pid",
				RegistryType: gcp.RegistryTypeLocalContainer,
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().CreateServiceAccount("pid", "cloud-controller", "cloud-controller").Return("email@sa", false, nil)

			err = bs.EnsureServiceAccounts()
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates both accounts for artifact registry", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:    "pid",
				RegistryType: gcp.RegistryTypeArtifactRegistry,
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().CreateServiceAccount("pid", "cloud-controller", "cloud-controller").Return("email@sa", false, nil)
			gc.EXPECT().CreateServiceAccount("pid", "artifact-registry-writer", "artifact-registry-writer").Return("writer@sa", true, nil)
			gc.EXPECT().CreateServiceAccountKey("pid", "writer@sa").Return("key-content", nil)

			err = bs.EnsureServiceAccounts()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.InstallConfig.Registry.Password).To(Equal("key-content"))
		})
	})

	Describe("Invalid cases", func() {
		It("fails when cloud-controller creation fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().CreateServiceAccount("pid", "cloud-controller", "cloud-controller").Return("", false, fmt.Errorf("create error"))

			err = bs.EnsureServiceAccounts()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("create error"))
		})
	})
})

var _ = Describe("EnsureIAMRoles", func() {
	Describe("Valid EnsureIAMRoles", func() {
		It("assigns roles correctly", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:    "pid",
				RegistryType: gcp.RegistryTypeArtifactRegistry,
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().AssignIAMRole("pid", "cloud-controller", "roles/compute.admin").Return(nil)
			gc.EXPECT().AssignIAMRole("pid", "artifact-registry-writer", "roles/artifactregistry.writer").Return(nil)

			err = bs.EnsureIAMRoles()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when AssignIAMRole fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().AssignIAMRole("pid", "cloud-controller", "roles/compute.admin").Return(fmt.Errorf("iam error"))

			err = bs.EnsureIAMRoles()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("iam error"))
		})
	})
})

var _ = Describe("EnsureVPC", func() {
	Describe("Valid EnsureVPC", func() {
		It("creates VPC, subnet, router, and nat", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().CreateVPC("pid", "us-central1", "pid-vpc", "pid-us-central1-subnet", "pid-router", "pid-nat-gateway").Return(nil)

			err = bs.EnsureVPC()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when CreateVPC fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().CreateVPC("pid", "us-central1", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("vpc error"))

			err = bs.EnsureVPC()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure VPC"))
			Expect(err.Error()).To(ContainSubstring("vpc error"))
		})
	})
})

var _ = Describe("EnsureFirewallRules", func() {
	Describe("Valid EnsureFirewallRules", func() {
		It("creates required firewall rules", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Expect 4 rules: allow-ssh-ext, allow-internal, allow-all-egress, allow-ingress-web, allow-ingress-postgres
			// Wait, code showed 5 blocks? ssh, internal, egress, web, postgres.
			gc.EXPECT().CreateFirewallRule("pid", mock.MatchedBy(func(r *computepb.Firewall) bool {
				return *r.Name == "allow-ssh-ext"
			})).Return(nil)
			gc.EXPECT().CreateFirewallRule("pid", mock.MatchedBy(func(r *computepb.Firewall) bool {
				return *r.Name == "allow-internal"
			})).Return(nil)
			gc.EXPECT().CreateFirewallRule("pid", mock.MatchedBy(func(r *computepb.Firewall) bool {
				return *r.Name == "allow-all-egress"
			})).Return(nil)
			gc.EXPECT().CreateFirewallRule("pid", mock.MatchedBy(func(r *computepb.Firewall) bool {
				return *r.Name == "allow-ingress-web"
			})).Return(nil)
			gc.EXPECT().CreateFirewallRule("pid", mock.MatchedBy(func(r *computepb.Firewall) bool {
				return *r.Name == "allow-ingress-postgres"
			})).Return(nil)

			err = bs.EnsureFirewallRules()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when first firewall rule creation fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().CreateFirewallRule("pid", mock.Anything).Return(fmt.Errorf("firewall error")).Once()

			err = bs.EnsureFirewallRules()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create jumpbox ssh firewall rule"))
		})
	})
})

var _ = Describe("EnsureComputeInstances", func() {
	Describe("Valid EnsureComputeInstances", func() {
		It("creates all instances", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:        "pid",
				Region:           "us-central1",
				Zone:             "us-central1-a",
				SSHPublicKeyPath: "key.pub",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Mock ReadFile for SSH key (called 9 times in parallel)
			fw.EXPECT().ReadFile("key.pub").Return([]byte("ssh-rsa AAA..."), nil).Times(9)

			// Mock CreateInstance (9 times)
			gc.EXPECT().CreateInstance("pid", "us-central1-a", mock.Anything).Return(nil).Times(9)

			// Mock GetInstance (9 times)
			ipResp := &computepb.Instance{
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						NetworkIP: protoString("10.0.0.x"),
						AccessConfigs: []*computepb.AccessConfig{
							{NatIP: protoString("1.2.3.x")},
						},
					},
				},
			}
			gc.EXPECT().GetInstance("pid", "us-central1-a", mock.Anything).Return(ipResp, nil).Times(9)

			// Mock UpdateNode (called once for jumpbox to update the original NodeManager)
			nm.EXPECT().UpdateNode(mock.Anything, mock.Anything, mock.Anything).Once()

			// Mock CreateSubNode (8 times for postgres, ceph, k0s - now in main goroutine after channel)
			nm.EXPECT().CreateSubNode(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(name, extIP, intIP string) node.NodeManager {
				m := node.NewMockNodeManager(GinkgoT())
				m.EXPECT().GetName().Return(name).Maybe() // Allow Name() calls for sorting
				return m
			}).Times(8)

			err = bs.EnsureComputeInstances()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(bs.Env.ControlPlaneNodes)).To(Equal(3))
			Expect(len(bs.Env.CephNodes)).To(Equal(4))
			Expect(bs.Env.PostgreSQLNode).NotTo(BeNil())
			Expect(bs.Env.Jumpbox).NotTo(BeNil()) // Jumpbox is now the NodeManager itself after UpdateNode
		})
	})

	Describe("Invalid cases", func() {
		It("fails when SSH key read fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:        "pid",
				Region:           "us-central1",
				Zone:             "us-central1-a",
				SSHPublicKeyPath: "key.pub",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().ReadFile("key.pub").Return(nil, fmt.Errorf("read error")).Maybe()

			err = bs.EnsureComputeInstances()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
		})

		It("fails when CreateInstance fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:        "pid",
				Region:           "us-central1",
				Zone:             "us-central1-a",
				SSHPublicKeyPath: "key.pub",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().ReadFile("key.pub").Return([]byte("ssh-rsa AAA..."), nil).Maybe()
			gc.EXPECT().CreateInstance("pid", "us-central1-a", mock.Anything).Return(fmt.Errorf("create error")).Maybe()

			err = bs.EnsureComputeInstances()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
		})

		It("fails when GetInstance fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID:        "pid",
				Region:           "us-central1",
				Zone:             "us-central1-a",
				SSHPublicKeyPath: "key.pub",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			fw.EXPECT().ReadFile("key.pub").Return([]byte("ssh-rsa AAA..."), nil).Maybe()
			gc.EXPECT().CreateInstance("pid", "us-central1-a", mock.Anything).Return(nil).Maybe()
			gc.EXPECT().GetInstance("pid", "us-central1-a", mock.Anything).Return(nil, fmt.Errorf("get error")).Maybe()

			err = bs.EnsureComputeInstances()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
		})
	})
})

var _ = Describe("EnsureGatewayIPAddresses", func() {
	Describe("Valid EnsureGatewayIPAddresses", func() {
		It("creates two addresses", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Gateway
			gc.EXPECT().GetAddress("pid", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("pid", "us-central1", mock.MatchedBy(func(a *computepb.Address) bool {
				return *a.Name == "gateway"
			})).Return("1.1.1.1", nil)

			// Public Gateway
			gc.EXPECT().GetAddress("pid", "us-central1", "public-gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("pid", "us-central1", mock.MatchedBy(func(a *computepb.Address) bool {
				return *a.Name == "public-gateway"
			})).Return("2.2.2.2", nil)

			err = bs.EnsureGatewayIPAddresses()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.GatewayIP).To(Equal("1.1.1.1"))
			Expect(bs.Env.PublicGatewayIP).To(Equal("2.2.2.2"))
		})
	})

	Describe("Invalid cases", func() {
		It("fails when gateway IP creation fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetAddress("pid", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("pid", "us-central1", mock.Anything).Return("", fmt.Errorf("create error"))

			err = bs.EnsureGatewayIPAddresses()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure gateway IP"))
		})

		It("fails when public gateway IP creation fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				ProjectID: "pid",
				Region:    "us-central1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().GetAddress("pid", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("pid", "us-central1", mock.MatchedBy(func(a *computepb.Address) bool {
				return *a.Name == "gateway"
			})).Return("1.1.1.1", nil)
			gc.EXPECT().GetAddress("pid", "us-central1", "public-gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("pid", "us-central1", mock.MatchedBy(func(a *computepb.Address) bool {
				return *a.Name == "public-gateway"
			})).Return("", fmt.Errorf("create error"))

			err = bs.EnsureGatewayIPAddresses()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure public gateway IP"))
		})
	})
})

var _ = Describe("EnsureRootLoginEnabled", func() {
	Describe("Valid EnsureRootLoginEnabled", func() {
		It("enables root login on all nodes", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			// Setup nodes
			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Use the same mock for all for simplicity, but expect multiple calls
			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm}
			bs.Env.CephNodes = []node.NodeManager{nm}

			// Total nodes: 1 (jumpbox) + 1 (pg) + 1 (cp) + 1 (ceph) = 4
			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().WaitForSSH(mock.Anything).Return(nil).Times(4)
			nm.EXPECT().HasRootLoginEnabled().Return(false).Times(4)
			nm.EXPECT().EnableRootLogin().Return(nil).Times(4)

			err = bs.EnsureRootLoginEnabled()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when WaitForSSH times out", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{}
			bs.Env.CephNodes = []node.NodeManager{}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().WaitForSSH(mock.Anything).Return(fmt.Errorf("timeout")).Once()

			err = bs.EnsureRootLoginEnabled()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out waiting for SSH service"))
		})

		It("fails when EnableRootLogin fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{}
			bs.Env.CephNodes = []node.NodeManager{}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().WaitForSSH(mock.Anything).Return(nil).Once()
			nm.EXPECT().HasRootLoginEnabled().Return(false).Once()
			nm.EXPECT().EnableRootLogin().Return(fmt.Errorf("enable error")).Times(3)

			err = bs.EnsureRootLoginEnabled()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to enable root login"))
		})
	})
})

var _ = Describe("EnsureJumpboxConfigured", func() {
	Describe("Valid EnsureJumpboxConfigured", func() {
		It("configures jumpbox", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm

			nm.EXPECT().HasAcceptEnvConfigured().Return(false)
			nm.EXPECT().ConfigureAcceptEnv().Return(nil)
			nm.EXPECT().HasCommand("oms-cli").Return(false)
			nm.EXPECT().InstallOms().Return(nil)

			err = bs.EnsureJumpboxConfigured()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when ConfigureAcceptEnv fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm

			nm.EXPECT().HasAcceptEnvConfigured().Return(false)
			nm.EXPECT().ConfigureAcceptEnv().Return(fmt.Errorf("config error"))

			err = bs.EnsureJumpboxConfigured()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to configure AcceptEnv"))
		})

		It("fails when InstallOms fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm

			nm.EXPECT().HasAcceptEnvConfigured().Return(true)
			nm.EXPECT().HasCommand("oms-cli").Return(false)
			nm.EXPECT().InstallOms().Return(fmt.Errorf("install error"))

			err = bs.EnsureJumpboxConfigured()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install OMS"))
		})
	})
})

var _ = Describe("EnsureHostsConfigured", func() {
	Describe("Valid EnsureHostsConfigured", func() {
		It("configures hosts", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Setup nodes
			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm}
			bs.Env.CephNodes = []node.NodeManager{} // Empty to reduce calls

			// Total nodes: 1 (pg) + 1 (cp) = 2
			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().HasInotifyWatchesConfigured().Return(false).Times(2)
			nm.EXPECT().ConfigureInotifyWatches().Return(nil).Times(2)
			nm.EXPECT().HasMemoryMapConfigured().Return(false).Times(2)
			nm.EXPECT().ConfigureMemoryMap().Return(nil).Times(2)

			err = bs.EnsureHostsConfigured()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when ConfigureInotifyWatches fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{}
			bs.Env.CephNodes = []node.NodeManager{}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().HasInotifyWatchesConfigured().Return(false)
			nm.EXPECT().ConfigureInotifyWatches().Return(fmt.Errorf("inotify error"))

			err = bs.EnsureHostsConfigured()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to configure inotify watches"))
		})

		It("fails when ConfigureMemoryMap fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.PostgreSQLNode = nm
			bs.Env.ControlPlaneNodes = []node.NodeManager{}
			bs.Env.CephNodes = []node.NodeManager{}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().HasInotifyWatchesConfigured().Return(true)
			nm.EXPECT().HasMemoryMapConfigured().Return(false)
			nm.EXPECT().ConfigureMemoryMap().Return(fmt.Errorf("memory map error"))

			err = bs.EnsureHostsConfigured()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to configure memory map"))
		})
	})
})

var _ = Describe("UpdateInstallConfig", func() {
	Describe("Valid UpdateInstallConfig", func() {
		It("updates config and writes files", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir:            "/secrets",
				InstallConfigPath:     "config.yaml",
				SecretsFilePath:       "secrets.yaml",
				DatacenterID:          1,
				BaseDomain:            "example.com",
				GatewayIP:             "1.1.1.1",
				PublicGatewayIP:       "2.2.2.2",
				GithubAppClientID:     "gh-id",
				GithubAppClientSecret: "gh-secret",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{},
					},
					Cluster: files.ClusterConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Setup Nodes
			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.CephNodes = []node.NodeManager{nm, nm, nm, nm}
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
			nm.EXPECT().GetExternalIP().Return("8.8.8.8").Maybe() // For PublicIP

			// Expectations
			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("config.yaml", true).Return(nil)
			icg.EXPECT().WriteVault("secrets.yaml", true).Return(nil)

			nm.EXPECT().CopyFile("config.yaml", "/etc/codesphere/config.yaml").Return(nil)
			nm.EXPECT().CopyFile("secrets.yaml", "/secrets/prod.vault.yaml").Return(nil)

			err = bs.UpdateInstallConfig()
			Expect(err).NotTo(HaveOccurred())

			Expect(bs.Env.InstallConfig.Datacenter.ID).To(Equal(1))
			Expect(bs.Env.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
		})
	})

	Describe("Invalid cases", func() {
		It("fails when GenerateSecrets fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir:            "/secrets",
				InstallConfigPath:     "config.yaml",
				SecretsFilePath:       "secrets.yaml",
				DatacenterID:          1,
				BaseDomain:            "example.com",
				GatewayIP:             "1.1.1.1",
				PublicGatewayIP:       "2.2.2.2",
				GithubAppClientID:     "gh-id",
				GithubAppClientSecret: "gh-secret",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{},
					},
					Cluster: files.ClusterConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.CephNodes = []node.NodeManager{nm, nm, nm, nm}
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
			nm.EXPECT().GetExternalIP().Return("8.8.8.8").Maybe()

			icg.EXPECT().GenerateSecrets().Return(fmt.Errorf("generate error"))

			err = bs.UpdateInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to generate secrets"))
		})

		It("fails when WriteInstallConfig fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir:            "/secrets",
				InstallConfigPath:     "config.yaml",
				SecretsFilePath:       "secrets.yaml",
				DatacenterID:          1,
				BaseDomain:            "example.com",
				GatewayIP:             "1.1.1.1",
				PublicGatewayIP:       "2.2.2.2",
				GithubAppClientID:     "gh-id",
				GithubAppClientSecret: "gh-secret",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{},
					},
					Cluster: files.ClusterConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.CephNodes = []node.NodeManager{nm, nm, nm, nm}
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
			nm.EXPECT().GetExternalIP().Return("8.8.8.8").Maybe()

			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("config.yaml", true).Return(fmt.Errorf("write error"))

			err = bs.UpdateInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write config file"))
		})

		It("fails when WriteVault fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir:            "/secrets",
				InstallConfigPath:     "config.yaml",
				SecretsFilePath:       "secrets.yaml",
				DatacenterID:          1,
				BaseDomain:            "example.com",
				GatewayIP:             "1.1.1.1",
				PublicGatewayIP:       "2.2.2.2",
				GithubAppClientID:     "gh-id",
				GithubAppClientSecret: "gh-secret",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{},
					},
					Cluster: files.ClusterConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.CephNodes = []node.NodeManager{nm, nm, nm, nm}
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
			nm.EXPECT().GetExternalIP().Return("8.8.8.8").Maybe()

			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("config.yaml", true).Return(nil)
			icg.EXPECT().WriteVault("secrets.yaml", true).Return(fmt.Errorf("vault write error"))

			err = bs.UpdateInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write vault file"))
		})

		It("fails when CopyFile config fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir:            "/secrets",
				InstallConfigPath:     "config.yaml",
				SecretsFilePath:       "secrets.yaml",
				DatacenterID:          1,
				BaseDomain:            "example.com",
				GatewayIP:             "1.1.1.1",
				PublicGatewayIP:       "2.2.2.2",
				GithubAppClientID:     "gh-id",
				GithubAppClientSecret: "gh-secret",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{},
					},
					Cluster: files.ClusterConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.CephNodes = []node.NodeManager{nm, nm, nm, nm}
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
			nm.EXPECT().GetExternalIP().Return("8.8.8.8").Maybe()

			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("config.yaml", true).Return(nil)
			icg.EXPECT().WriteVault("secrets.yaml", true).Return(nil)
			nm.EXPECT().CopyFile("config.yaml", "/etc/codesphere/config.yaml").Return(fmt.Errorf("copy error"))

			err = bs.UpdateInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy install config to jumpbox"))
		})

		It("fails when CopyFile secrets fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir:            "/secrets",
				InstallConfigPath:     "config.yaml",
				SecretsFilePath:       "secrets.yaml",
				DatacenterID:          1,
				BaseDomain:            "example.com",
				GatewayIP:             "1.1.1.1",
				PublicGatewayIP:       "2.2.2.2",
				GithubAppClientID:     "gh-id",
				GithubAppClientSecret: "gh-secret",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{},
					},
					Cluster: files.ClusterConfig{},
				},
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.Jumpbox = nm
			bs.Env.PostgreSQLNode = nm
			bs.Env.CephNodes = []node.NodeManager{nm, nm, nm, nm}
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetName().Return("mock-node").Maybe()
			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()
			nm.EXPECT().GetExternalIP().Return("8.8.8.8").Maybe()

			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("config.yaml", true).Return(nil)
			icg.EXPECT().WriteVault("secrets.yaml", true).Return(nil)
			nm.EXPECT().CopyFile("config.yaml", "/etc/codesphere/config.yaml").Return(nil)
			nm.EXPECT().CopyFile("secrets.yaml", "/secrets/prod.vault.yaml").Return(fmt.Errorf("copy error"))

			err = bs.UpdateInstallConfig()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy secrets file to jumpbox"))
		})
	})
})

var _ = Describe("EnsureAgeKey", func() {
	Describe("Valid EnsureAgeKey", func() {
		It("generates key if missing", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir: "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().HasFile("/secrets/age_key.txt").Return(false)
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "age-keygen")
			}), true).Return(nil)

			err = bs.EnsureAgeKey()
			Expect(err).NotTo(HaveOccurred())
		})

		It("skips if key exists", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir: "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().HasFile("/secrets/age_key.txt").Return(true)
			// No SSH command expected

			err = bs.EnsureAgeKey()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when age-keygen command fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir: "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().HasFile("/secrets/age_key.txt").Return(false)
			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "age-keygen")
			}), true).Return(fmt.Errorf("keygen error"))

			err = bs.EnsureAgeKey()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to generate age key on jumpbox"))
		})
	})
})

var _ = Describe("EncryptVault", func() {
	Describe("Valid EncryptVault", func() {
		It("encrypts vault using sops", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir: "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.HasPrefix(cmd, "cp ")
			}), true).Return(nil)

			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "sops --encrypt")
			}), true).Return(nil)

			err = bs.EncryptVault()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when backup vault command fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir: "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.HasPrefix(cmd, "cp ")
			}), true).Return(fmt.Errorf("backup error"))

			err = bs.EncryptVault()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed backup vault on jumpbox"))
		})

		It("fails when sops encrypt command fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				SecretsDir: "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.HasPrefix(cmd, "cp ")
			}), true).Return(nil)

			nm.EXPECT().RunSSHCommand("root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "sops --encrypt")
			}), true).Return(fmt.Errorf("encrypt error"))

			err = bs.EncryptVault()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to encrypt vault on jumpbox"))
		})
	})
})

var _ = Describe("EnsureDNSRecords", func() {
	Describe("Valid EnsureDNSRecords", func() {
		It("ensures DNS records", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				DNSProjectID:    "dns-proj",
				DNSZoneName:     "zone",
				BaseDomain:      "example.com",
				GatewayIP:       "1.1.1.1",
				PublicGatewayIP: "2.2.2.2",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().EnsureDNSManagedZone("dns-proj", "zone", "example.com.", mock.Anything).Return(nil)
			gc.EXPECT().EnsureDNSRecordSets("dns-proj", "zone", mock.MatchedBy(func(records []*dns.ResourceRecordSet) bool {
				// Expect 4 records: *.ws, *.cs, cs, ws
				return len(records) == 4
			})).Return(nil)

			err = bs.EnsureDNSRecords()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when EnsureDNSManagedZone fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				DNSProjectID:    "dns-proj",
				DNSZoneName:     "zone",
				BaseDomain:      "example.com",
				GatewayIP:       "1.1.1.1",
				PublicGatewayIP: "2.2.2.2",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().EnsureDNSManagedZone("dns-proj", "zone", "example.com.", mock.Anything).Return(fmt.Errorf("zone error"))

			err = bs.EnsureDNSRecords()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure DNS managed zone"))
		})

		It("fails when EnsureDNSRecordSets fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				DNSProjectID:    "dns-proj",
				DNSZoneName:     "zone",
				BaseDomain:      "example.com",
				GatewayIP:       "1.1.1.1",
				PublicGatewayIP: "2.2.2.2",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			gc.EXPECT().EnsureDNSManagedZone("dns-proj", "zone", "example.com.", mock.Anything).Return(nil)
			gc.EXPECT().EnsureDNSRecordSets("dns-proj", "zone", mock.Anything).Return(fmt.Errorf("record error"))

			err = bs.EnsureDNSRecords()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure DNS record sets"))
		})
	})
})

var _ = Describe("InstallCodesphere", func() {
	Describe("Valid InstallCodesphere", func() {
		It("downloads and installs codesphere", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallCodesphereVersion: "v1.2.3",
				SecretsDir:               "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			// Expect download package
			nm.EXPECT().RunSSHCommand("root", "oms-cli download package v1.2.3", true).Return(nil)

			// Expect install codesphere
			nm.EXPECT().RunSSHCommand("root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /secrets/age_key.txt -p v1.2.3.tar.gz", true).Return(nil)

			err = bs.InstallCodesphere()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when download package fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallCodesphereVersion: "v1.2.3",
				SecretsDir:               "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().RunSSHCommand("root", "oms-cli download package v1.2.3", true).Return(fmt.Errorf("download error"))

			err = bs.InstallCodesphere()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download Codesphere package from jumpbox"))
		})

		It("fails when install codesphere fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				InstallCodesphereVersion: "v1.2.3",
				SecretsDir:               "/secrets",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())
			bs.Env.Jumpbox = nm

			nm.EXPECT().RunSSHCommand("root", "oms-cli download package v1.2.3", true).Return(nil)
			nm.EXPECT().RunSSHCommand("root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /secrets/age_key.txt -p v1.2.3.tar.gz", true).Return(fmt.Errorf("install error"))

			err = bs.InstallCodesphere()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install Codesphere from jumpbox"))
		})
	})
})

var _ = Describe("GenerateK0sConfigScript", func() {
	Describe("Valid GenerateK0sConfigScript", func() {
		It("generates script", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				PublicGatewayIP: "2.2.2.2",
				GatewayIP:       "1.1.1.1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			// Setup required nodes (indices 0, 1, 2 accessed)
			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()

			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
			nm.EXPECT().CopyFile("configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
			nm.EXPECT().RunSSHCommand("root", "chmod +x /root/configure-k0s.sh", true).Return(nil)

			err = bs.GenerateK0sConfigScript()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Invalid cases", func() {
		It("fails when WriteFile fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				PublicGatewayIP: "2.2.2.2",
				GatewayIP:       "1.1.1.1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()

			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(fmt.Errorf("write error"))

			err = bs.GenerateK0sConfigScript()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write configure-k0s.sh"))
		})

		It("fails when CopyFile fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				PublicGatewayIP: "2.2.2.2",
				GatewayIP:       "1.1.1.1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()

			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
			nm.EXPECT().CopyFile("configure-k0s.sh", "/root/configure-k0s.sh").Return(fmt.Errorf("copy error"))

			err = bs.GenerateK0sConfigScript()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy configure-k0s.sh to control plane node"))
		})

		It("fails when RunSSHCommand chmod fails", func() {
			env := env.NewEnv()
			ctx := context.Background()
			csEnv := &gcp.CodesphereEnvironment{
				PublicGatewayIP: "2.2.2.2",
				GatewayIP:       "1.1.1.1",
			}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())
			nm := node.NewMockNodeManager(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, env, stlog, csEnv, icg, gc, nm, fw)
			Expect(err).NotTo(HaveOccurred())

			bs.Env.ControlPlaneNodes = []node.NodeManager{nm, nm, nm}

			nm.EXPECT().GetInternalIP().Return("10.0.0.1").Maybe()

			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
			nm.EXPECT().CopyFile("configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
			nm.EXPECT().RunSSHCommand("root", "chmod +x /root/configure-k0s.sh", true).Return(fmt.Errorf("chmod error"))

			err = bs.GenerateK0sConfigScript()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to make configure-k0s.sh executable"))
		})
	})
})
