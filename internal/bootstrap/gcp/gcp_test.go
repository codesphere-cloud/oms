// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"strings"

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

func jumpbboxMatcher(node *node.Node) bool {
	return node.GetName() == "jumpbox"
}

var _ = Describe("GCP Bootstrapper", func() {
	var (
		nodeClient *node.MockNodeClient
		csEnv      *gcp.CodesphereEnvironment
		ctx        context.Context
		e          env.Env
	)

	BeforeEach(func() {
		nodeClient = node.NewMockNodeClient(GinkgoT())
		ctx = context.Background()
		e = env.NewEnv()

		csEnv = &gcp.CodesphereEnvironment{
			InstallConfigPath: "fake-config-file",
			SecretsFilePath:   "fake-secret",
			ProjectName:       "test-project",
			SecretsDir:        "/etc/codesphere/secrets",
			BillingAccount:    "test-billing-account",
			Region:            "us-central1",
			Zone:              "us-central1-a",
			DatacenterID:      1,
			BaseDomain:        "example.com",
			DNSProjectID:      "dns-project",
			DNSZoneName:       "test-zone",
			ProjectID:         "pid",
			InstallConfig: &files.RootConfig{
				Registry: &files.RegistryConfig{},
				Postgres: files.PostgresConfig{
					Primary: &files.PostgresPrimaryConfig{},
				},
				Cluster: files.ClusterConfig{},
			},
			Jumpbox:           fakeNode("jumpbox", nodeClient),
			PostgreSQLNode:    fakeNode("postgres", nodeClient),
			ControlPlaneNodes: []*node.Node{fakeNode("k0s-1", nodeClient), fakeNode("k0s-2", nodeClient), fakeNode("k0s-3", nodeClient)},
			CephNodes:         []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient), fakeNode("ceph-3", nodeClient), fakeNode("ceph-4", nodeClient)},
		}
	})
	Describe("NewGCPBootstrapper", func() {
		It("creates a valid GCPBootstrapper", func() {
			csEnv = &gcp.CodesphereEnvironment{}
			stlog := bootstrap.NewStepLogger(false)

			icg := installer.NewMockInstallConfigManager(GinkgoT())
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw := util.NewMockFileIO(GinkgoT())

			bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(bs).NotTo(BeNil())
		})
	})

	Describe("Bootstrap", func() {
		var (
			icg *installer.MockInstallConfigManager
			gc  *gcp.MockGCPClientManager
			fw  *util.MockFileIO
			bs  *gcp.GCPBootstrapper
		)

		BeforeEach(func() {
			stlog := bootstrap.NewStepLogger(false)

			icg = installer.NewMockInstallConfigManager(GinkgoT())
			gc = gcp.NewMockGCPClientManager(GinkgoT())
			fw = util.NewMockFileIO(GinkgoT())

			csEnv = &gcp.CodesphereEnvironment{
				InstallConfigPath: "fake-config-file",
				SecretsFilePath:   "fake-secret",
				SecretsDir:        "/etc/codesphere/secrets",
				ProjectName:       "test-project",
				BillingAccount:    "test-billing-account",
				Region:            "us-central1",
				Zone:              "us-central1-a",
				BaseDomain:        "example.com",
				DNSProjectID:      "dns-project",
				DNSZoneName:       "test-zone",
				InstallConfig: &files.RootConfig{
					Registry: &files.RegistryConfig{},
				},
			}
			var err error
			bs, err = gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs bootstrap successfully", func() {
			bs.Env.RegistryType = gcp.RegistryTypeArtifactRegistry
			bs.Env.WriteConfig = true

			// 1. EnsureInstallConfig
			fw.EXPECT().Exists("fake-config-file").Return(false)
			icg.EXPECT().ApplyProfile("dev").Return(nil)
			// Returning a real install config to avoid nil pointer dereferences later
			icg.EXPECT().GetInstallConfig().RunAndReturn(func() *files.RootConfig {
				realIcm := installer.NewInstallConfigManager()
				_ = realIcm.ApplyProfile("dev")
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

			// 12. EnsureGatewayIPAddresses
			gc.EXPECT().GetAddress("test-project-id", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("test-project-id", "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "gateway" })).Return("1.1.1.1", nil)
			gc.EXPECT().GetAddress("test-project-id", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().GetAddress("test-project-id", "us-central1", "public-gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress("test-project-id", "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "public-gateway" })).Return("2.2.2.2", nil)
			gc.EXPECT().GetAddress("test-project-id", "us-central1", "public-gateway").Return(&computepb.Address{Address: protoString("2.2.2.2")}, nil)

			// 16. UpdateInstallConfig
			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "fake-config-file", "/etc/codesphere/config.yaml").Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "fake-secret", "/etc/codesphere/secrets/prod.vault.yaml").Return(nil)
			icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

			// Enable Root Login
			nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil).Return(nil)
			nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

			// 17. EnsureAgeKey
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(nil)
			nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)

			// 18. EncryptVault
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "cp /etc/codesphere/secrets/prod.vault.yaml{,.bak}").Return(nil)
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "sops --encrypt")
			})).Return(nil)

			// 19. EnsureDNSRecords
			gc.EXPECT().EnsureDNSManagedZone("dns-project", "test-zone", "example.com.", mock.Anything).Return(nil)
			gc.EXPECT().EnsureDNSRecordSets("dns-project", "test-zone", mock.MatchedBy(func(records []*dns.ResourceRecordSet) bool {
				return len(records) == 4
			})).Return(nil)

			// 20. GenerateK0sConfigScript
			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
			nodeClient.EXPECT().RunCommand(mock.Anything, "root", "chmod +x /root/configure-k0s.sh").Return(nil)

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
			Expect(bs.Env.Jumpbox.GetName()).To(Equal("jumpbox"))
			Expect(bs.Env.Jumpbox.GetExternalIP()).To(Equal("1.2.3.4"))
			Expect(bs.Env.Jumpbox.GetInternalIP()).To(Equal("10.0.0.1"))

			Expect(bs.Env.PostgreSQLNode.GetName()).To(Equal("postgres"))
			Expect(bs.Env.PostgreSQLNode.GetExternalIP()).To(Equal("1.2.3.4"))
			Expect(bs.Env.PostgreSQLNode.GetInternalIP()).To(Equal("10.0.0.1"))

			for _, cephNode := range bs.Env.CephNodes {
				Expect(cephNode.GetName()).To(MatchRegexp("ceph-\\d+"))
				Expect(cephNode.GetExternalIP()).To(Equal("1.2.3.4"))
				Expect(cephNode.GetInternalIP()).To(Equal("10.0.0.1"))
			}

			for _, cpNode := range bs.Env.ControlPlaneNodes {
				Expect(cpNode.GetName()).To(MatchRegexp("k0s-\\d+"))
				Expect(cpNode.GetExternalIP()).To(Equal("1.2.3.4"))
				Expect(cpNode.GetInternalIP()).To(Equal("10.0.0.1"))
			}
		})
	})

	Describe("EnsureInstallConfig", func() {
		Describe("Valid EnsureInstallConfig", func() {
			It("uses existing when config file exists", func() {
				csEnv = &gcp.CodesphereEnvironment{
					InstallConfigPath: "existing-config-file",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().Exists("existing-config-file").Return(true)
				icg.EXPECT().LoadInstallConfigFromFile("existing-config-file").Return(nil)
				icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

				err = bs.EnsureInstallConfig()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates install config when missing", func() {
				csEnv = &gcp.CodesphereEnvironment{
					InstallConfigPath: "nonexistent-config-file",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					InstallConfigPath: "existing-bad-config",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().Exists("existing-bad-config").Return(true)
				icg.EXPECT().LoadInstallConfigFromFile("existing-bad-config").Return(fmt.Errorf("bad format"))

				err = bs.EnsureInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load config file"))
				Expect(err.Error()).To(ContainSubstring("bad format"))
			})

			It("returns error when config file missing and applying profile fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					InstallConfigPath: "missing-config",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureSecrets", func() {
		Describe("Valid EnsureSecrets", func() {
			It("loads existing secrets file", func() {
				csEnv = &gcp.CodesphereEnvironment{
					SecretsFilePath: "existing-secrets",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().Exists("existing-secrets").Return(true)
				icg.EXPECT().LoadVaultFromFile("existing-secrets").Return(nil)
				icg.EXPECT().MergeVaultIntoConfig().Return(nil)
				icg.EXPECT().GetVault().Return(&files.InstallVault{})

				err = bs.EnsureSecrets()
				Expect(err).NotTo(HaveOccurred())
			})

			It("skips when secrets file missing", func() {
				csEnv = &gcp.CodesphereEnvironment{
					SecretsFilePath: "missing-secrets",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().Exists("missing-secrets").Return(false)
				icg.EXPECT().GetVault().Return(&files.InstallVault{})

				err = bs.EnsureSecrets()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("returns error when secrets file load fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					SecretsFilePath: "bad-secrets",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().Exists("bad-secrets").Return(true)
				icg.EXPECT().LoadVaultFromFile("bad-secrets").Return(fmt.Errorf("load error"))

				err = bs.EnsureSecrets()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load vault file"))
				Expect(err.Error()).To(ContainSubstring("load error"))
			})

			It("returns error when merge fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					SecretsFilePath: "merr-secrets",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureProject", func() {
		Describe("Valid EnsureProject", func() {
			It("uses existing project", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectName: "existing-proj",
					FolderID:    "123",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().GetProjectByName("123", "existing-proj").Return(&resourcemanagerpb.Project{ProjectId: "existing-id", Name: "existing-proj"}, nil)

				err = bs.EnsureProject()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.ProjectID).To(Equal("existing-id"))
			})

			It("creates project when missing", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectName: "new-proj",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					ProjectName: "error-proj",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().GetProjectByName("", "error-proj").Return(nil, fmt.Errorf("api error"))

				err = bs.EnsureProject()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get project"))
				Expect(err.Error()).To(ContainSubstring("api error"))
			})

			It("returns error when CreateProject fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectName: "fail-create-proj",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureBilling", func() {
		Describe("Valid EnsureBilling", func() {
			It("does nothing if billing already enabled correctly", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:      "pid",
					BillingAccount: "billing-123",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:      "pid",
					BillingAccount: "billing-123",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().GetBillingInfo("pid").Return(nil, fmt.Errorf("billing info error"))

				err = bs.EnsureBilling()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get billing info"))
				Expect(err.Error()).To(ContainSubstring("billing info error"))
			})

			It("fails when EnableBilling fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:      "pid",
					BillingAccount: "acc",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureAPIsEnabled", func() {
		Describe("Valid EnsureAPIsEnabled", func() {
			It("enables default APIs", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().EnableAPIs("pid", mock.Anything).Return(fmt.Errorf("api error"))

				err = bs.EnsureAPIsEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to enable APIs"))
				Expect(err.Error()).To(ContainSubstring("api error"))
			})
		})
	})

	Describe("EnsureArtifactRegistry", func() {
		Describe("Valid EnsureArtifactRegistry", func() {
			It("uses existing registry if present", func() {
				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				repo := &artifactregistrypb.Repository{Name: "projects/pid/locations/us-central1/repositories/codesphere-registry"}
				gc.EXPECT().GetArtifactRegistry("pid", "us-central1", "codesphere-registry").Return(repo, nil)

				err = bs.EnsureArtifactRegistry()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates registry if missing", func() {
				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureLocalContainerRegistry", func() {
		Describe("Valid EnsureLocalContainerRegistry", func() {
			It("installs local registry", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					InstallConfig: &files.RootConfig{
						Registry: &files.RegistryConfig{},
					},
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				// Setup mocked node
				bs.Env.PostgreSQLNode = fakeNode("postgres", nodeClient)

				// Check if running - return error to simulate not running
				nodeClient.EXPECT().RunCommand(bs.Env.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// Install commands (8 commands) + scp/update-ca/docker commands (3 per 4 nodes = 12)
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil).Times(8 + 12)

				bs.Env.ControlPlaneNodes = []*node.Node{fakeNode("k0s-1", nodeClient), fakeNode("k0s-2", nodeClient)}
				bs.Env.CephNodes = []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient)}

				err = bs.EnsureLocalContainerRegistry()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.InstallConfig.Registry.Username).To(Equal("custom-registry"))
			})
		})

		Describe("Invalid cases", func() {
			var (
				ctx context.Context
				icg *installer.MockInstallConfigManager
				gc  *gcp.MockGCPClientManager
				fw  *util.MockFileIO
				bs  *gcp.GCPBootstrapper
			)

			BeforeEach(func() {
				ctx = context.Background()
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					InstallConfig: &files.RootConfig{
						Registry: &files.RegistryConfig{},
					},
					PostgreSQLNode:    fakeNode("postgres", nodeClient),
					ControlPlaneNodes: []*node.Node{fakeNode("k0s-1", nodeClient), fakeNode("k0s-2", nodeClient)},
					CephNodes:         []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient)},
				}
				stlog := bootstrap.NewStepLogger(false)

				icg = installer.NewMockInstallConfigManager(GinkgoT())
				gc = gcp.NewMockGCPClientManager(GinkgoT())
				fw = util.NewMockFileIO(GinkgoT())

				var err error
				bs, err = gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when the 8th install command fails", func() {
				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// First 7 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(7)

				// 8th install command fails
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(fmt.Errorf("ssh error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("ssh error"))
			})

			It("fails when the first scp command fails", func() {
				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// All 8 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(8)

				// First scp command fails
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "scp ")
				})).Return(fmt.Errorf("scp error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy registry certificate"))
			})

			It("fails when update-ca-certificates fails", func() {
				// Override node setup for this test
				bs.Env.ControlPlaneNodes = []*node.Node{fakeNode("k0s-1", nodeClient)}
				bs.Env.CephNodes = []*node.Node{}

				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// All 8 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(8)
				// scp succeeds
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "scp ")
				})).Return(nil).Once()

				// update-ca-certificates fails
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "update-ca-certificates").Return(fmt.Errorf("ca update error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update CA certificates"))
			})

			It("fails when docker restart fails", func() {
				// Override node setup for this test
				bs.Env.ControlPlaneNodes = []*node.Node{fakeNode("k0s-1", nodeClient)}
				bs.Env.CephNodes = []*node.Node{}

				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// All 8 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(8)

				// scp succeeds
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "scp ")
				})).Return(nil).Once()

				// update-ca-certificates succeeds
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "update-ca-certificates").Return(nil).Once()

				// docker restart fails
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "systemctl restart docker.service || true").Return(fmt.Errorf("docker restart error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to restart docker service"))
			})
		})
	})

	Describe("EnsureServiceAccounts", func() {
		Describe("Valid EnsureServiceAccounts", func() {
			It("creates cloud-controller and skips writer if not artifact registry", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:    "pid",
					RegistryType: gcp.RegistryTypeLocalContainer,
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().CreateServiceAccount("pid", "cloud-controller", "cloud-controller").Return("email@sa", false, nil)

				err = bs.EnsureServiceAccounts()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates both accounts for artifact registry", func() {
				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().CreateServiceAccount("pid", "cloud-controller", "cloud-controller").Return("", false, fmt.Errorf("create error"))

				err = bs.EnsureServiceAccounts()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("create error"))
			})
		})
	})

	Describe("EnsureIAMRoles", func() {
		Describe("Valid EnsureIAMRoles", func() {
			It("assigns roles correctly", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:    "pid",
					RegistryType: gcp.RegistryTypeArtifactRegistry,
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().AssignIAMRole("pid", "cloud-controller", "roles/compute.admin").Return(nil)
				gc.EXPECT().AssignIAMRole("pid", "artifact-registry-writer", "roles/artifactregistry.writer").Return(nil)

				err = bs.EnsureIAMRoles()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when AssignIAMRole fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().AssignIAMRole("pid", "cloud-controller", "roles/compute.admin").Return(fmt.Errorf("iam error"))

				err = bs.EnsureIAMRoles()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("iam error"))
			})
		})
	})

	Describe("EnsureVPC", func() {
		Describe("Valid EnsureVPC", func() {
			It("creates VPC, subnet, router, and nat", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					Region:    "us-central1",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().CreateVPC("pid", "us-central1", "pid-vpc", "pid-us-central1-subnet", "pid-router", "pid-nat-gateway").Return(nil)

				err = bs.EnsureVPC()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when CreateVPC fails", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					Region:    "us-central1",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().CreateVPC("pid", "us-central1", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("vpc error"))

				err = bs.EnsureVPC()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure VPC"))
				Expect(err.Error()).To(ContainSubstring("vpc error"))
			})
		})
	})

	Describe("EnsureFirewallRules", func() {
		Describe("Valid EnsureFirewallRules", func() {
			It("creates required firewall rules", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().CreateFirewallRule("pid", mock.Anything).Return(fmt.Errorf("firewall error")).Once()

				err = bs.EnsureFirewallRules()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create jumpbox ssh firewall rule"))
			})
		})
	})

	Describe("EnsureComputeInstances", func() {
		Describe("Valid EnsureComputeInstances", func() {
			It("creates all instances", func() {
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:        "pid",
					Region:           "us-central1",
					Zone:             "us-central1-a",
					SSHPublicKeyPath: "key.pub",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

				err = bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(bs.Env.ControlPlaneNodes)).To(Equal(3))
				Expect(len(bs.Env.CephNodes)).To(Equal(4))
				Expect(bs.Env.PostgreSQLNode).NotTo(BeNil())
				Expect(bs.Env.Jumpbox).NotTo(BeNil())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when SSH key read fails", func() {

				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:        "pid",
					Region:           "us-central1",
					Zone:             "us-central1-a",
					SSHPublicKeyPath: "key.pub",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().ReadFile("key.pub").Return(nil, fmt.Errorf("read error")).Maybe()

				err = bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})

			It("fails when CreateInstance fails", func() {

				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:        "pid",
					Region:           "us-central1",
					Zone:             "us-central1-a",
					SSHPublicKeyPath: "key.pub",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().ReadFile("key.pub").Return([]byte("ssh-rsa AAA..."), nil).Maybe()
				gc.EXPECT().CreateInstance("pid", "us-central1-a", mock.Anything).Return(fmt.Errorf("create error")).Maybe()

				err = bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})

			It("fails when GetInstance fails", func() {

				csEnv = &gcp.CodesphereEnvironment{
					ProjectID:        "pid",
					Region:           "us-central1",
					Zone:             "us-central1-a",
					SSHPublicKeyPath: "key.pub",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureGatewayIPAddresses", func() {
		Describe("Valid EnsureGatewayIPAddresses", func() {
			It("creates two addresses", func() {

				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					Region:    "us-central1",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					Region:    "us-central1",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().GetAddress("pid", "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateAddress("pid", "us-central1", mock.Anything).Return("", fmt.Errorf("create error"))

				err = bs.EnsureGatewayIPAddresses()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure gateway IP"))
			})

			It("fails when public gateway IP creation fails", func() {

				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					Region:    "us-central1",
				}
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

	Describe("EnsureRootLoginEnabled", func() {
		Context("When WaitReady times out", func() {
			It("fails", func() {
				nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(fmt.Errorf("TIMEOUT!"))

				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureRootLoginEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out waiting for SSH service"))
			})
		})
		Context("When WaitReady succeeds", func() {
			BeforeEach(func() {
				nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil)
			})
			Describe("Valid EnsureRootLoginEnabled", func() {
				It("enables root login on all nodes", func() {
					nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(nil)
					stlog := bootstrap.NewStepLogger(false)

					icg := installer.NewMockInstallConfigManager(GinkgoT())
					gc := gcp.NewMockGCPClientManager(GinkgoT())
					fw := util.NewMockFileIO(GinkgoT())

					// Setup nodes
					bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
					Expect(err).NotTo(HaveOccurred())

					err = bs.EnsureRootLoginEnabled()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			It("fails when EnableRootLogin fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(fmt.Errorf("ouch"))
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureRootLoginEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to enable root login"))
			})
		})
	})

	Describe("EnsureJumpboxConfigured", func() {
		Describe("Valid EnsureJumpboxConfigured", func() {
			It("configures jumpbox", func() {
				// Setup jumpbox node requires some commands to run
				nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureJumpboxConfigured()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when ConfigureAcceptEnv fails", func() {
				// Setup jumpbox node requires some commands to run
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(fmt.Errorf("ouch")).Twice()

				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureJumpboxConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure AcceptEnv"))
			})

			It("fails when InstallOms fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(nil)
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("outch"))
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureJumpboxConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install OMS"))
			})
		})
	})

	Describe("EnsureHostsConfigured", func() {
		Describe("Valid EnsureHostsConfigured", func() {
			It("configures hosts", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil)
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureHostsConfigured()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when ConfigureInotifyWatches fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("ouch"))
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure inotify watches"))
			})

			It("fails when ConfigureMemoryMap fails", func() {
				stlog := bootstrap.NewStepLogger(false)
				mock.InOrder(
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil).Times(1),                // for inotify
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("ouch")).Times(2), // for memory map
				)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure memory map"))
			})
		})
	})

	Describe("UpdateInstallConfig", func() {
		Describe("Valid UpdateInstallConfig", func() {
			It("updates config and writes files", func() {

				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				// Expectations
				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

				err = bs.UpdateInstallConfig()
				Expect(err).NotTo(HaveOccurred())

				Expect(bs.Env.InstallConfig.Datacenter.ID).To(Equal(1))
				Expect(bs.Env.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
			})
		})

		Describe("Invalid cases", func() {
			It("fails when GenerateSecrets fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				icg.EXPECT().GenerateSecrets().Return(fmt.Errorf("generate error"))

				err = bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to generate secrets"))
			})

			It("fails when WriteInstallConfig fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(fmt.Errorf("write error"))

				err = bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write config file"))
			})

			It("fails when WriteVault fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(fmt.Errorf("vault write error"))

				err = bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write vault file"))
			})

			It("fails when CopyFile config fails", func() {

				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("copy error")).Once()

				err = bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy install config to jumpbox"))
			})

			It("fails when CopyFile secrets fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, "fake-config-file", mock.Anything).Return(nil).Once()
				nodeClient.EXPECT().CopyFile(mock.Anything, "fake-secret", mock.Anything).Return(fmt.Errorf("copy error")).Once()

				err = bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy secrets file to jumpbox"))
			})
		})
	})

	Describe("EnsureAgeKey", func() {
		Describe("Valid EnsureAgeKey", func() {
			It("generates key if missing", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(nil)

				err = bs.EnsureAgeKey()
				Expect(err).NotTo(HaveOccurred())
			})

			It("skips if key exists", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(true)
				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureAgeKey()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when age-keygen command fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(fmt.Errorf("ouch"))

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				err = bs.EnsureAgeKey()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to generate age key on jumpbox"))
			})
		})
	})

	Describe("EncryptVault", func() {
		Describe("Valid EncryptVault", func() {
			It("encrypts vault using sops", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "cp ")
				})).Return(nil)

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "sops --encrypt")
				})).Return(nil)

				err = bs.EncryptVault()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when backup vault command fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "cp ")
				})).Return(fmt.Errorf("backup error"))

				err = bs.EncryptVault()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed backup vault on jumpbox"))
			})

			It("fails when sops encrypt command fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())
				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "cp ")
				})).Return(nil)

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "sops --encrypt")
				})).Return(fmt.Errorf("encrypt error"))

				err = bs.EncryptVault()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to encrypt vault on jumpbox"))
			})
		})
	})

	Describe("EnsureDNSRecords", func() {
		Describe("Valid EnsureDNSRecords", func() {
			It("ensures DNS records", func() {

				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
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

				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().EnsureDNSManagedZone("dns-proj", "zone", "example.com.", mock.Anything).Return(fmt.Errorf("zone error"))

				err = bs.EnsureDNSRecords()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure DNS managed zone"))
			})

			It("fails when EnsureDNSRecordSets fails", func() {

				csEnv = &gcp.CodesphereEnvironment{
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

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				gc.EXPECT().EnsureDNSManagedZone("dns-proj", "zone", "example.com.", mock.Anything).Return(nil)
				gc.EXPECT().EnsureDNSRecordSets("dns-proj", "zone", mock.Anything).Return(fmt.Errorf("record error"))

				err = bs.EnsureDNSRecords()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure DNS record sets"))
			})
		})
	})

	Describe("InstallCodesphere", func() {
		BeforeEach(func() {
			csEnv.InstallCodesphereVersion = "v1.2.3"
		})
		Describe("Valid InstallCodesphere", func() {
			It("downloads and installs codesphere", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				// Expect download package
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package v1.2.3").Return(nil)

				// Expect install codesphere
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt -p v1.2.3.tar.gz").Return(nil)

				err = bs.InstallCodesphere()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when download package fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package v1.2.3").Return(fmt.Errorf("download error"))

				err = bs.InstallCodesphere()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to download Codesphere package from jumpbox"))
			})

			It("fails when install codesphere fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package v1.2.3").Return(nil)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt -p v1.2.3.tar.gz").Return(fmt.Errorf("install error"))

				err = bs.InstallCodesphere()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install Codesphere from jumpbox"))
			})
		})
	})

	Describe("GenerateK0sConfigScript", func() {
		Describe("Valid GenerateK0sConfigScript", func() {
			It("generates script", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
				nodeClient.EXPECT().CopyFile(bs.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "chmod +x /root/configure-k0s.sh").Return(nil)

				err = bs.GenerateK0sConfigScript()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when WriteFile fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(fmt.Errorf("write error"))

				err = bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write configure-k0s.sh"))
			})

			It("fails when CopyFile fails", func() {
				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
				nodeClient.EXPECT().CopyFile(mock.Anything, "configure-k0s.sh", "/root/configure-k0s.sh").Return(fmt.Errorf("copy error"))

				err = bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy configure-k0s.sh to control plane node"))
			})

			It("fails when RunSSHCommand chmod fails", func() {

				stlog := bootstrap.NewStepLogger(false)

				icg := installer.NewMockInstallConfigManager(GinkgoT())
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				fw := util.NewMockFileIO(GinkgoT())

				bs, err := gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient)
				Expect(err).NotTo(HaveOccurred())

				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)

				nodeClient.EXPECT().CopyFile(bs.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "chmod +x /root/configure-k0s.sh").Return(fmt.Errorf("chmod error"))

				err = bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to make configure-k0s.sh executable"))
			})
		})
	})

})

func fakeNode(name string, commandRunner node.NodeClient) *node.Node {
	return &node.Node{
		Name:       name,
		ExternalIP: "1.2.3.4",
		InternalIP: "10.0.0.1",

		NodeClient: commandRunner,
	}
}
